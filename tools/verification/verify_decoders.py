#!/usr/bin/env python3
"""Independent development-only ZXing-C++ and ZXing Java decoding.

Checks complete real PNG pixels before decoding. Java strict PNG detection uses
scale 3, without PURE_BARCODE or matrix fallback. The known default-scale8
case and an independent same-pixel control are reported separately, including
both rejections if they occur. No failed detection is counted as success.
"""
# SPDX-License-Identifier: MIT
# Adapted from SpecQR-CPP tools/verify-decode.py at e91cd8.
import argparse
import base64
import hashlib
import importlib
import importlib.metadata
import json
import shutil
from pathlib import Path
import string
import struct
import subprocess
import sys
import tempfile
import time
import urllib.request
import zlib
from protocol import generate, matrix_hash, configure, DRIVER, verify_identity
from verify_conformance import snapshot, tool_snapshot, node_identity

TOOLS = Path(__file__).resolve().parent
ROOT = TOOLS.parents[1]
_FAILURE_OUTPUT = None
_FAILURE_CONTEXT = {}
JAR_VERSION = '3.5.4'
JAR_SHA256 = '71de5d89341b5fcf5dd89da7f44e84d825d0e084cdf3ec77c9abe26b0f0ceb13'
JAR_URL = f'https://repo.maven.apache.org/maven2/com/google/zxing/core/{JAR_VERSION}/core-{JAR_VERSION}.jar'


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def b64(data):
    return base64.b64encode(bytes(data)).decode('ascii')


def verify_png(encoded, matrix, scale):
    """Decode standard RGB/RGBA PNG scanlines and prove every resulting pixel.

    Go's image/png optimizes opaque NRGBA to RGB and chooses adaptive filters.
    Neither storage choice relaxes the expected RGBA pixels or quiet zone.
    """
    png = bytes.fromhex(encoded)
    require(png[:8] == b'\x89PNG\r\n\x1a\n', 'PNG signature mismatch')
    position, compressed, header, ended = 8, bytearray(), None, False
    while position < len(png):
        require(position + 12 <= len(png), 'Truncated PNG chunk')
        length = struct.unpack('>I', png[position:position + 4])[0]
        require(position + length + 12 <= len(png), 'Truncated PNG chunk data')
        kind, data = png[position + 4:position + 8], png[position + 8:position + 8 + length]
        require(zlib.crc32(kind + data) == struct.unpack('>I', png[position + 8 + length:position + 12 + length])[0], 'PNG CRC mismatch')
        if kind == b'IHDR':
            require(header is None and position == 8 and length == 13, 'PNG header structure mismatch')
            header = struct.unpack('>IIBBBBB', data)
        else:
            require(header is not None, 'PNG header must precede image data')
            if kind == b'IDAT':
                compressed.extend(data)
            elif kind == b'IEND':
                require(length == 0 and position + 12 == len(png), 'PNG end/trailing data mismatch')
                ended = True
            elif kind[:1].isupper():
                raise RuntimeError('Unexpected critical PNG chunk: ' + repr(kind))
        position += length + 12
    require(header is not None and ended and compressed, 'Incomplete PNG')
    width, height, depth, color, compression, filtering, interlace = header
    require(depth == 8 and color in (2, 6) and (compression, filtering, interlace) == (0, 0, 0), 'PNG RGB/RGBA format mismatch')
    dimension = (len(matrix) + 8) * scale
    require(width == height == dimension, 'PNG dimensions mismatch')
    channels = 3 if color == 2 else 4
    stride = dimension * channels
    inflater = zlib.decompressobj()
    expected_length = dimension * (stride + 1)
    raw = inflater.decompress(compressed, expected_length + 1)
    require(inflater.eof and not inflater.unused_data and not inflater.unconsumed_tail and len(raw) == expected_length, 'PNG scanline length/compression mismatch')
    white = (b'\xff' * channels) * scale
    black = (b'\x00\x00\x00' + (b'\xff' if channels == 4 else b'')) * scale
    blank = white * (len(matrix) + 8)
    rows = [blank] * 4 + [white * 4 + b''.join(black if v == '1' else white for v in row) + white * 4 for row in matrix] + [blank] * 4
    luminance, previous = bytearray(), bytearray(stride)
    for y in range(dimension):
        start = y * (stride + 1)
        filter_type, scanline = raw[start], bytearray(raw[start + 1:start + stride + 1])
        require(filter_type in range(5), 'Invalid PNG scanline filter')
        if filter_type == 1:
            for x in range(channels, stride):
                scanline[x] = (scanline[x] + scanline[x - channels]) & 255
        elif filter_type == 2:
            scanline = bytearray((value + above) & 255 for value, above in zip(scanline, previous)) if any(scanline) else previous[:]
        elif filter_type == 3:
            for x in range(stride):
                left = scanline[x - channels] if x >= channels else 0
                scanline[x] = (scanline[x] + (left + previous[x]) // 2) & 255
        elif filter_type == 4:
            for x in range(stride):
                left = scanline[x - channels] if x >= channels else 0
                above = previous[x]
                upper_left = previous[x - channels] if x >= channels else 0
                p = left + above - upper_left
                distances = (abs(p - left), abs(p - above), abs(p - upper_left))
                predictor = (left, above, upper_left)[distances.index(min(distances))]
                scanline[x] = (scanline[x] + predictor) & 255
        # RGB storage has implicit alpha 255 at every pixel; RGBA must explicitly
        # match it. The complete channel comparison validates both representations.
        require(scanline == rows[y // scale], f'PNG pixel/quiet-zone mismatch at row {y}')
        luminance.extend(scanline[0::channels])
        previous = scanline
    return png, luminance, dimension


def independent_png(matrix, scale):
    dimension = (len(matrix) + 8) * scale
    raw = bytearray()
    for y in range(dimension):
        raw.append(0)
        for x in range(dimension):
            row, column = y // scale - 4, x // scale - 4
            raw.append(0 if 0 <= row < len(matrix) and 0 <= column < len(matrix) and matrix[row][column] == '1' else 255)
    def chunk(kind, data):
        return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind + data))
    return b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', dimension, dimension, 8, 0, 0, 0, 0)) + chunk(b'IDAT', zlib.compress(raw)) + chunk(b'IEND', b'')


def verify_control_pixels(png, expected_luminance, dimension):
    position, compressed = 8, bytearray()
    while position < len(png):
        length = struct.unpack('>I', png[position:position + 4])[0]
        kind, data = png[position + 4:position + 8], png[position + 8:position + 8 + length]
        require(zlib.crc32(kind + data) == struct.unpack('>I', png[position + 8 + length:position + 12 + length])[0], 'Independent control PNG CRC mismatch')
        if kind == b'IHDR':
            require(struct.unpack('>IIBBBBB', data) == (dimension, dimension, 8, 0, 0, 0, 0), 'Independent control PNG format mismatch')
        if kind == b'IDAT':
            compressed.extend(data)
        position += length + 12
    raw = zlib.decompress(compressed)
    require(len(raw) == dimension * (dimension + 1), 'Independent control scanline length differs')
    for y in range(dimension):
        row = raw[y * (dimension + 1):(y + 1) * (dimension + 1)]
        require(row[0] == 0 and row[1:] == expected_luminance[y * dimension:(y + 1) * dimension], 'Independent control pixels differ from Go PNG')

def cases(include_second):
    result = []
    def add(name, request, expected):
        result.append((name, request, expected))
    for level in 'LMQH':
        for mask in range(8):
            options = {'version': 4, 'errorCorrectionLevel': level, 'maskPattern': mask}
            for mode, text in [('numeric', '012345678901234567890123456789'), ('alphanumeric', 'SPECQR / 12345 %'), ('kanji', '漢字東京大阪日本語')]:
                add(f'{mode}-{level}-{mask}', {'text': text, 'options': {**options, 'mode': mode}}, {'text': text, 'bytes': text.encode('shift_jis' if mode == 'kanji' else 'utf-8')})
            text = 'e\u0301🙂漢字 café'
            add(f'utf8-{level}-{mask}', {'text': text, 'options': {**options, 'mode': 'byte', 'eci': 26}}, {'text': text, 'bytes': text.encode(), 'identifier': ']Q2'})
            data = bytes([0, 255, 128, 127, 13, 10, 29, 0, 254, 65])
            add(f'binary-{level}-{mask}', {'bytes': list(data), 'options': options}, {'bytes': data})
            add(f'fnc1-alpha-{level}-{mask}', {'segments': [{'mode': 'fnc1'}, {'mode': 'alphanumeric', 'text': '10LOT%21SER%%IAL'}], 'options': options}, {'text': '10LOT\x1d21SER%IAL', 'bytes': b'10LOT\x1d21SER%IAL', 'identifier': ']Q3'})
            add(f'fnc1-high-{level}-{mask}', {'text': '10LOT%\x1d21SER%%IAL', 'options': {**options, 'gs1': True}}, {'text': '10LOT%\x1d21SER%%IAL', 'bytes': b'10LOT%\x1d21SER%%IAL', 'identifier': ']Q3'})
    for assignment, data, text in [(3, [99,97,102,233], 'café'), (20,[138,191,142,154],'漢字'), (170,[65,83,67,73,73],'ASCII')]:
        add(f'eci-{assignment}', {'segments':[{'mode':'eci','assignmentNumber':assignment},{'mode':'byte','bytes':data}]}, {'text':text,'bytes':bytes(data),'identifier':']Q2'})
    for total in range(2, 17):
        for index in range(1, total + 1):
            data, checksum = bytes([0, index, total, 255, 128]), (total * 17 + index * 31) & 255
            add(f'sa-{index}-{total}', {'bytes': list(data), 'options': {'version': 2, 'maskPattern': index % 8, 'structuredAppend': {'index': index, 'total': total, 'parity': checksum}}}, {'bytes': data, 'sequence': (index - 1) * 16 + total - 1, 'parity': checksum})
    if include_second:
        indicators = [f'{n:02}' for n in range(100)] + list(string.ascii_uppercase + string.ascii_lowercase)
        for i, indicator in enumerate(indicators):
            for route in ['manual', 'options']:
                request = {'options': {'version': 3, 'errorCorrectionLevel': 'LMQH'[i % 4], 'maskPattern': i % 8}}
                if route == 'manual':
                    request['segments'] = [{'mode': 'fnc1-second', 'applicationIndicator': indicator}, {'mode': 'alphanumeric', 'text': 'ABC%123%%XYZ'}]
                    data = b'ABC\x1d123%XYZ'
                else:
                    request.update(text='ABC%123%%XYZ'); request['options']['fnc1Second'] = indicator
                    data = b'ABC%123%%XYZ'
                add(f'second-{indicator}-{route}', request, {'bytes': indicator.encode() + data, 'identifier': ']Q5'})
    # Detection and real PNG coverage across every Model 2 version. Fixed
    # deterministic payload size grows independently of the encoder's estimates.
    for version in range(1, 41):
        data = bytes((i * 149 + version * 43) & 255 for i in range(min(5 + version * 5, 200)))
        add(f'version-{version}', {'bytes': list(data), 'options': {'version': version, 'errorCorrectionLevel': 'LMQH'[(version - 1) % 4], 'maskPattern': (version - 1) % 8}}, {'bytes': data})
    return result


def main():
    global _FAILURE_OUTPUT, _FAILURE_CONTEXT
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--candidate', type=Path, default=ROOT)
    p.add_argument('--go', default=None)
    p.add_argument('--binary', type=Path)
    p.add_argument('--decoder', choices=['cpp', 'java'], required=True)
    p.add_argument('--dependency-dir', type=Path, required=True)
    p.add_argument('--java', default='java')
    p.add_argument('--png-scale', type=int, choices=[3, 8], help='Java scale 8 runs the complete isolated default-scale diagnostic; strict Java scope defaults to scale 3')
    p.add_argument('--node', default='node')
    p.add_argument('--no-install', action='store_true')
    p.add_argument('--output', type=Path)
    args = p.parse_args()
    if args.decoder == 'cpp' and args.png_scale not in (None, 8):
        p.error('ZXing-C++ strict PNG scope requires scale 8')
    configure(args.go, args.binary)
    args.output = args.output or ROOT / 'artifacts' / f'zxing-{args.decoder}.json'
    args.output.parent.mkdir(parents=True, exist_ok=True)
    _FAILURE_OUTPUT = args.output
    _FAILURE_CONTEXT = {'status': 'running', 'decoder': args.decoder, 'candidate': str(args.candidate), 'scopeCompleted': False}
    work = args.output.parent / f'zxing-{args.decoder}-fixtures'
    work.mkdir(parents=True, exist_ok=True)
    args.dependency_dir.mkdir(parents=True, exist_ok=True)
    started = time.perf_counter()
    source_hashes = snapshot(args.candidate, 'all')
    tool_hashes = tool_snapshot()
    node_dependencies = node_identity(args.node, ['nayuki-qr-code-generator']) if args.decoder == 'cpp' else None
    nonce = 'decoder-' + str(time.time_ns())
    candidate_identity = generate(args.candidate, [{'command': 'identity', 'nonce': nonce}])[0]
    verify_identity(args.candidate, candidate_identity, nonce)
    require('error' not in candidate_identity, 'Go candidate load failed')
    adapter_sha = hashlib.sha256(DRIVER.read_bytes()).hexdigest()
    decoder_identity = {}
    if args.decoder == 'cpp':
        requirements = TOOLS / 'zxing-cpp-requirements.txt'
        requirement_hash = hashlib.sha256(requirements.read_bytes()).hexdigest()
        receipt = args.dependency_dir / '.verified-requirements-sha256'
        if not receipt.exists() or receipt.read_text().strip() != requirement_hash:
            require(not args.no_install, 'Hash-verified ZXing-C++ wheels not present; no tests ran')
            subprocess.run([sys.executable, '-m', 'pip', 'install', '--target', str(args.dependency_dir), '--upgrade', '--only-binary=:all:', '--no-deps', '--require-hashes', '-r', str(requirements)], check=True)
            receipt.write_text(requirement_hash + '\n')
        sys.path.insert(0, str(args.dependency_dir.resolve()))
        zxing = importlib.import_module('zxingcpp')
        require(importlib.metadata.version('zxing-cpp') == '3.1.1', 'Unexpected ZXing-C++ version')
        # ZXing-C++ 3.1.1's public module is a Python wrapper. Fingerprint the
        # actual native decoder, not only that wrapper, and recheck both at exit.
        native = importlib.import_module('zxingcpp.zxingcpp')
        decoder_files = [Path(zxing.__file__).resolve(), Path(native.__file__).resolve()]
        dependency_hashes = {str(path): hashlib.sha256(path.read_bytes()).hexdigest() for path in decoder_files}
        decoder_identity = {'decoder': 'ZXing-C++ 3.1.1', 'requirementsSha256': requirement_hash,
                            'extensionSha256': dependency_hashes[str(Path(native.__file__).resolve())],
                            'loadedDecoderFilesSha256': dependency_hashes}
    else:
        jar = args.dependency_dir / f'core-{JAR_VERSION}.jar'
        if not jar.exists():
            require(not args.no_install, 'Pinned ZXing Java JAR not present; no tests ran')
            with urllib.request.urlopen(JAR_URL, timeout=60) as response:
                data = response.read(4 * 1024 * 1024 + 1)
            require(hashlib.sha256(data).hexdigest() == JAR_SHA256, 'Downloaded Java JAR hash mismatch')
            jar.write_bytes(data)
        require(hashlib.sha256(jar.read_bytes()).hexdigest() == JAR_SHA256, 'Java JAR hash mismatch')
        java_command = shutil.which(args.java)
        require(java_command is not None, 'Java executable was not found')
        java_executable = Path(java_command).resolve()
        args.java = str(java_executable)
        java_sha = hashlib.sha256(java_executable.read_bytes()).hexdigest()
        java_runtime = subprocess.check_output([args.java, '--version'], text=True).splitlines()
        decoder_identity = {'decoder': 'ZXing Java 3.5.4', 'jarSha256': JAR_SHA256,
                            'java': java_runtime[0], 'javaRuntime': java_runtime,
                            'javaExecutable': str(java_executable), 'javaExecutableSha256': java_sha}
    scale = args.png_scale or (8 if args.decoder == 'cpp' else 3)
    tests, inputs, expectations = cases(args.decoder == 'cpp'), [], []
    counts = {'matrixDecodes': 0, 'pngDecodes': 0, 'verifiedRgbaPixels': 0, 'structuredAppendHeaders': 0, 'fnc1SecondSymbols': 0, 'damagedSymbolsCorrected': 0, 'eciTextModeChecks': 0}
    failures = []
    _FAILURE_CONTEXT.update(counts=counts, failures=failures, goProcessIdentity=candidate_identity, sourceSha256=source_hashes, testToolSha256=tool_hashes)
    negative_controls = []
    decoded_versions = {'matrix': set(), 'png': set()}
    png_details, corpus_checks, corpus_report = {}, [], None
    for offset in range(0, len(tests), 12):
        batch = tests[offset:offset + 12]
        output = generate(args.candidate, [{**r, 'pngScale': scale} for _, r, _ in batch])
        for (name, request, expected), symbol in zip(batch, output):
            require('error' not in symbol, f'Generation failed {name}: {symbol}')
            png, luminance, dimension = verify_png(symbol['png'], symbol['matrix'], scale)
            counts['verifiedRgbaPixels'] += dimension * dimension
            counts['structuredAppendHeaders'] += int(name.startswith('sa-'))
            counts['fnc1SecondSymbols'] += int(name.startswith('second-'))
            if args.decoder == 'cpp':
                width = (len(symbol['matrix']) + 8) * 3
                pixels = bytearray()
                for y in range(width):
                    for x in range(width):
                        my, mx = y // 3 - 4, x // 3 - 4
                        pixels.append(0 if 0 <= my < len(symbol['matrix']) and 0 <= mx < len(symbol['matrix']) and symbol['matrix'][my][mx] == '1' else 255)
                for route, data, size in [('matrix', pixels, width), ('png', luminance, dimension)]:
                    decoded = zxing.read_barcode(memoryview(data).cast('B', shape=(size, size)), text_mode=zxing.TextMode.Plain)
                    # ZXing-C++ exposes the base identifier through this property even
                    # when ECI exists. ECI mode exposes the effective ]Q2 identifier
                    # in text and transcodes payload to UTF-8. Check both APIs.
                    identifier = expected.get('identifier', ']Q1')
                    property_identifier = ']Q1' if identifier == ']Q2' else identifier
                    ok = decoded is not None and decoded.valid and decoded.bytes == expected['bytes'] and decoded.symbology_identifier == property_identifier and decoded.ec_level == symbol['ecc'] and decoded.extra['Version'] == str(symbol['version']) and decoded.extra['DataMask'] == symbol['mask']
                    if identifier == ']Q2':
                        eci_decoded = zxing.read_barcode(memoryview(data).cast('B', shape=(size, size)), text_mode=zxing.TextMode.ECI)
                        eci_ok = eci_decoded is not None and eci_decoded.valid and eci_decoded.text == ']Q2\\000026' + expected['text']
                        ok = ok and eci_ok
                        counts['eciTextModeChecks'] += int(eci_ok)
                    if ok:
                        counts[route + 'Decodes'] += 1
                        decoded_versions[route].add(symbol['version'])
                    else:
                        failures.append({'case': name, 'route': route, 'decoded': None if decoded is None else {'bytes': decoded.bytes.hex(), 'identifier': decoded.symbology_identifier, 'ecc': decoded.ec_level}, 'expectedHex': expected['bytes'].hex()})
            else:
                expected = {**expected, 'data': symbol['data'], 'ecc': symbol['ecc'], 'version': symbol['version']}
                if scale == 8:
                    png_details[name] = {'matrix': symbol['matrix'], 'version': symbol['version'], 'expected': expected}
                for route, value in [('matrix', ','.join(symbol['matrix'])), ('png', str((work / (name + '.png')).resolve()))]:
                    if route == 'png':
                        Path(value).write_bytes(png)
                    inputs.append(f'{route}\t{value}')
                    expectations.append((name, route, expected))
    # Read metadata and reconstruct complete high-level Structured Append sets.
    groups = [
        ('numeric', {'text': '0123456789' * 13, 'options': {'version': 1, 'mode': 'numeric'}}, ('0123456789' * 13).encode()),
        ('alphanumeric', {'text': 'SPECQR / 12345 ' * 10, 'options': {'version': 2, 'mode': 'alphanumeric'}}, ('SPECQR / 12345 ' * 10).encode()),
        ('unicode', {'text': 'e\u0301🙂漢字' * 12, 'options': {'version': 2, 'mode': 'byte'}}, ('e\u0301🙂漢字' * 12).encode()),
        ('all-bytes', {'bytes': list(range(256)), 'options': {'version': 2}}, bytes(range(256))),
        ('sixteen', {'bytes': list(range(240)), 'options': {'version': 1, 'errorCorrectionLevel': 'L'}}, bytes(range(240)))
    ]
    group_checks = []
    counts.update(highLevelSets=0, highLevelSymbols=0, independentReconstructions=0)
    for name, request, source in groups:
        result = generate(args.candidate, [{**request, 'command': 'structured-append', 'pngScale': scale}])[0]
        require('error' not in result, f'Structured Append generation failed: {name}: {result}')
        checksum = 0
        for byte in source:
            checksum ^= byte
        require(result['parity'] == checksum and 2 <= result['total'] <= 16 and len(result['symbols']) == result['total'], 'SA source parity/count differs')
        if name == 'sixteen':
            require(result['total'] == 16, 'Maximum-count SA fixture did not generate 16 members')
        indices, parts = [], []
        for i, symbol in enumerate(result['symbols']):
            png, luminance, dimension = verify_png(symbol['png'], symbol['matrix'], scale)
            counts['verifiedRgbaPixels'] += dimension * dimension
            counts['highLevelSymbols'] += 1
            if args.decoder == 'cpp':
                decoded = zxing.read_barcode(memoryview(luminance).cast('B', shape=(dimension, dimension)), text_mode=zxing.TextMode.Plain)
                require(decoded is not None and decoded.valid, f'SA PNG detection failed {name}-{i}')
                parts.append(decoded.bytes)
                counts['pngDecodes'] += 1
            else:
                expected = {'data': symbol['data'], 'ecc': symbol['ecc'], 'version': symbol['version'], 'sequence': i * 16 + result['total'] - 1,
                            'parity': checksum, 'identifier': ']Q2' if request['options'].get('eci') == 26 else ']Q1'}
                if scale == 8:
                    png_details[f'high-{name}-{i}'] = {'matrix': symbol['matrix'], 'version': symbol['version'], 'expected': expected}
                indices.append(len(expectations))
                for route, value in [('matrix', ','.join(symbol['matrix'])), ('png', str((work / f'high-{name}-{i}.png').resolve()))]:
                    if route == 'png':
                        Path(value).write_bytes(png)
                    inputs.append(f'{route}\t{value}')
                    expectations.append((f'high-{name}-{i}', route, expected))
        if args.decoder == 'cpp':
            require(b''.join(parts) == source, f'SA reconstruction failed {name}')
            counts['independentReconstructions'] += 1
        else:
            group_checks.append((name, indices, source, 'bytes' in request))
        counts['highLevelSets'] += 1
    # Three distinct data codewords in version 1, independently corrected.
    if args.decoder == 'java':
        requests = [{'text': 'ECC', 'includeMatrix': True, 'options': {'version': 1, 'mode': 'byte', 'errorCorrectionLevel': level, 'maskPattern': mask}} for level in 'LMQH' for mask in range(8)]
        for i, symbol in enumerate(generate(args.candidate, requests)):
            damaged = [list(row) for row in symbol['matrix']]
            for y in [20, 16, 12]:
                damaged[y][20] = '0' if damaged[y][20] == '1' else '1'
            inputs.append('matrix\t' + ','.join(''.join(row) for row in damaged))
            expectations.append((f'damage-{i}', 'damage', {'text': 'ECC', 'data': symbol['data'], 'ecc': symbol['ecc'], 'errorsCorrected': 3}))
    # Genuine undecodable matrix/PNG controls; never counted as successful decodes.
    blank_matrix = ['0' * 21] * 21
    if args.decoder == 'java':
        blank_path = (work / 'blank-negative-control.png').resolve()
        blank_path.write_bytes(independent_png(blank_matrix, 3))
        inputs.extend(['matrix\t' + ','.join(blank_matrix), 'png\t' + str(blank_path)])
        expectations.extend([('blank-matrix', 'negative', {'error': True}),
                             ('blank-png', 'negative', {'error': True})])
    else:
        for route, blank_scale in [('matrix', 3), ('png', 8)]:
            dimension = 29 * blank_scale
            pixels = bytearray([255]) * (dimension * dimension)
            if route == 'png':
                verify_control_pixels(independent_png(blank_matrix, blank_scale), pixels, dimension)
            decoded = zxing.read_barcode(memoryview(pixels).cast('B', shape=(dimension, dimension)), text_mode=zxing.TextMode.Plain)
            rejected = decoded is None or not decoded.valid
            negative_controls.append({'case': 'blank-' + route, 'rejected': rejected})
            if not rejected:
                failures.append({'case': 'blank-' + route, 'error': 'Undecodable control was accepted'})
    # Keep the exact default-scale8 detector regression and independent control.
    request = {'text': 'SPECQR / 12345 %', 'options': {'version': 4, 'errorCorrectionLevel': 'L', 'maskPattern': 0, 'mode': 'alphanumeric'}, 'pngScale': 8}
    diagnostic = generate(args.candidate, [request])[0]
    png, luminance, dimension = verify_png(diagnostic['png'], diagnostic['matrix'], 8)
    control = independent_png(diagnostic['matrix'], 8)
    verify_control_pixels(control, luminance, dimension)
    diagnostic_report = {'case': 'alphanumeric-L-0', 'scale': 8, 'quietZone': 4, 'allPixelsMatch': True, 'independentControlPixelsIdentical': True,
                         'goPngSha256': hashlib.sha256(png).hexdigest(), 'controlPngSha256': hashlib.sha256(control).hexdigest(),
                         'matrixSha256': matrix_hash(diagnostic['matrix']), 'countedAsStrictSuccess': False}
    if args.decoder == 'java':
        for name, data in [('java-candidate-default-scale8', png), ('independent-default-scale8', control)]:
            path = (work / (name + '.png')).resolve(); path.write_bytes(data); inputs.append(f'png\t{path}')
        input_path = work / 'inputs.tsv'; input_path.write_text('\n'.join(inputs) + '\n')
        output = subprocess.check_output([args.java, '-XX:ActiveProcessorCount=2', '-Djava.awt.headless=true', '--class-path', str(jar.resolve()), str(TOOLS / 'zxing-java' / 'DecodeSymbols.java'), str(input_path.resolve())], text=True)
        (work / 'decoded.jsonl').write_text(output)
        records = [json.loads(line) for line in output.splitlines()]
        require(len(records) == len(inputs), 'Java response count mismatch')
        for record, (name, route, expected) in zip(records, expectations):
            if expected.get('error'):
                rejected = 'error' in record
                negative_controls.append({'case': name, 'rejected': rejected, 'actual': record})
                if not rejected:
                    failures.append({'case': name, 'route': route, 'error': 'Undecodable control was accepted'})
                continue
            ok = 'error' not in record
            if ok:
                ok = record['rawBytesBase64'] == b64(bytes.fromhex(expected['data'])) and record['ecc'] == expected['ecc'] and record['symbologyIdentifier'] == expected.get('identifier', ']Q1') and record['sequence'] == expected.get('sequence') and record['parity'] == expected.get('parity')
                if 'text' in expected:
                    ok = ok and base64.b64decode(record['textBase64']).decode() == expected['text']
                elif 'bytes' in expected:
                    ok = ok and b''.join(base64.b64decode(v) for v in record['byteSegments']) == expected['bytes']
            if 'errorsCorrected' in expected:
                ok = ok and record.get('errorsCorrected') == expected['errorsCorrected']
            if scale == 8 and route == 'png':
                corpus_checks.append({'case': name, 'version': expected['version'], 'passed': ok,
                                      'outcome': {k: v for k, v in record.items() if k != 'ordinal'}})
            if ok:
                counts['damagedSymbolsCorrected' if route == 'damage' else route + 'Decodes'] += 1
                if route in decoded_versions and 'version' in expected:
                    decoded_versions[route].add(expected['version'])
            else:
                failures.append({'case': name, 'route': route, 'actual': record})
        if scale == 8:
            # Retain every default-scale failure and test a separately encoded,
            # pixel-identical control for each one. Controls never replace a
            # candidate result or count toward either successful-decoder total.
            control_inputs, control_checks = [], []
            for check in corpus_checks:
                if check['passed']:
                    continue
                name, details = check['case'], png_details[check['case']]
                candidate_png = (work / (name + '.png')).read_bytes()
                _, luminance, dimension = verify_png(candidate_png.hex(), details['matrix'], scale)
                control_png = independent_png(details['matrix'], scale)
                verify_control_pixels(control_png, luminance, dimension)
                control_path = (work / (name + '-pixel-identical-control.png')).resolve()
                control_path.write_bytes(control_png)
                control_inputs.append('png\t' + str(control_path))
                control_checks.append({'case': name, 'version': details['version'],
                                       'candidateOutcome': check['outcome'],
                                       'candidatePngSha256': hashlib.sha256(candidate_png).hexdigest(),
                                       'controlPngSha256': hashlib.sha256(control_png).hexdigest(),
                                       'matrixSha256': matrix_hash(details['matrix']),
                                       'independentControlPixelsIdentical': True, 'countedAsStrictSuccess': False})
            if control_inputs:
                control_input_path = work / 'default-scale8-control-inputs.tsv'
                control_input_path.write_text('\n'.join(control_inputs) + '\n')
                control_output = subprocess.check_output([args.java, '-XX:ActiveProcessorCount=2', '-Djava.awt.headless=true', '--class-path', str(jar.resolve()), str(TOOLS / 'zxing-java' / 'DecodeSymbols.java'), str(control_input_path.resolve())], text=True)
                (work / 'default-scale8-control-decoded.jsonl').write_text(control_output)
                control_records = [json.loads(line) for line in control_output.splitlines()]
                require(len(control_records) == len(control_inputs), 'Default-scale8 control response count mismatch')
                for check, record in zip(control_checks, control_records):
                    outcome = {k: v for k, v in record.items() if k != 'ordinal'}
                    check.update(controlOutcome=outcome, matchingOutcomes=outcome == check['candidateOutcome'])
                    if not check['matchingOutcomes']:
                        failures.append({'case': check['case'], 'route': 'default-scale8-control', 'error': 'Candidate differs from pixel-identical independent control'})
            by_version = {}
            for check in corpus_checks:
                version = by_version.setdefault(str(check['version']), {'attempted': 0, 'successes': 0, 'failures': 0})
                version['attempted'] += 1
                version['successes' if check['passed'] else 'failures'] += 1
            require(len(corpus_checks) == 446, 'Default-scale8 diagnostic corpus count changed')
            corpus_report = {'scope': 'isolated complete Java default-scale8 diagnostic',
                             'status': 'failed' if control_checks else 'passed',
                             'attempted': len(corpus_checks), 'successes': sum(check['passed'] for check in corpus_checks),
                             'failures': len(control_checks), 'byVersion': by_version,
                             'failureControls': control_checks, 'countedAsStrictScale3Success': False,
                             'allFailureControlsPixelIdentical': all(check['independentControlPixelsIdentical'] for check in control_checks),
                             'allFailureControlOutcomesMatch': all(check['matchingOutcomes'] for check in control_checks)}
        for name, indices, source, binary in group_checks:
            for route in [0, 1]:
                parts = sorted((records[i + route] for i in reversed(indices)), key=lambda r: r.get('sequence', -1))
                reconstructed = (b''.join(base64.b64decode(v) for part in parts for v in part.get('byteSegments', [])) if binary
                                 else b''.join(base64.b64decode(part.get('textBase64', '')) for part in parts))
                if reconstructed == source:
                    counts['independentReconstructions'] += 1
                else:
                    failures.append({'case': 'high-' + name, 'route': ['matrix', 'png'][route], 'error': 'independent reconstruction mismatch'})
        outcomes = [{k: v for k, v in r.items() if k != 'ordinal'} for r in records[-2:]]
        diagnostic_report.update(outcomes=outcomes, matchingOutcomes=outcomes[0] == outcomes[1],
                                 detectionFailures=sum('error' in r for r in outcomes), detectionSuccesses=sum('error' not in r for r in outcomes))
        require(outcomes[0] == outcomes[1], 'Default-scale PNG differs from same-pixel independent control')
        for outcome in outcomes:
            if 'error' not in outcome:
                require(base64.b64decode(outcome['textBase64']).decode() == request['text'] and outcome['rawBytesBase64'] == b64(bytes.fromhex(diagnostic['data'])) and outcome['ecc'] == 'L' and outcome['symbologyIdentifier'] == ']Q1', 'Default-scale diagnostic decoded incorrect payload/metadata')
    else:
        decoded = zxing.read_barcode(memoryview(luminance).cast('B', shape=(dimension, dimension)), text_mode=zxing.TextMode.Plain)
        ok = decoded is not None and decoded.valid and decoded.bytes == request['text'].encode()
        diagnostic_report.update(exactPayloadDecoded=ok, detectionFailures=int(not ok), detectionSuccesses=int(ok))
        require(ok, 'ZXing-C++ default-scale8 PNG diagnostic failed')
    eci_controls = []
    if args.decoder == 'cpp':
        controls = subprocess.check_output([args.node, str(TOOLS / 'eci-control.mjs')], text=True)
        for control in map(json.loads, controls.splitlines()):
            request = {'includeMatrix': True, 'segments': [{'mode': 'eci', 'assignmentNumber': control['assignment']}, {'mode': 'byte', 'bytes': control['bytes']}],
                       'options': {'version': 4, 'errorCorrectionLevel': 'M', 'maskPattern': 0}}
            symbol = generate(args.candidate, [request])[0]
            require(symbol.get('matrix') == control['matrix'], 'Go ECI control differs from independent Nayuki')
            width = (len(control['matrix']) + 8) * 3
            pixels = bytearray()
            for y in range(width):
                for x in range(width):
                    my, mx = y // 3 - 4, x // 3 - 4
                    pixels.append(0 if 0 <= my < len(control['matrix']) and 0 <= mx < len(control['matrix']) and control['matrix'][my][mx] == '1' else 255)
            image = memoryview(pixels).cast('B', shape=(width, width))
            plain = zxing.read_barcode(image, text_mode=zxing.TextMode.Plain)
            eci = zxing.read_barcode(image, text_mode=zxing.TextMode.ECI)
            require(plain is not None and eci is not None and plain.valid and eci.valid, 'Independent ECI control detection failed')
            require(plain.bytes == bytes(control['bytes']) and plain.symbology_identifier == ']Q1', 'Independent ECI base API differs')
            require(eci.symbology_identifier == ']Q1' and eci.text == ']Q2\\000026' + control['text'], 'Independent ECI text API differs')
            eci_controls.append({'source': 'nayuki-qr-code-generator@1.8.0', 'assignment': control['assignment'],
                                 'matrixSha256': matrix_hash(control['matrix']), 'goMatrixIdentical': True,
                                 'decodedBytesHex': plain.bytes.hex(), 'plainIdentifier': plain.symbology_identifier,
                                 'eciIdentifierProperty': eci.symbology_identifier, 'eciText': eci.text})
        (args.output.parent / 'zxing-cpp-eci-controls.json').write_text(json.dumps({'status': 'passed', 'controls': eci_controls}, indent=2, ensure_ascii=False) + '\n')
    for route, versions in decoded_versions.items():
        missing = sorted(set(range(1, 41)) - versions)
        if missing:
            failures.append({'route': route, 'missingDecodedVersions': missing})
    if len(negative_controls) != 2 or not all(control['rejected'] for control in negative_controls):
        failures.append({'error': 'Decoder negative-control coverage or rejection mismatch'})
    expected_counts = ({'matrixDecodes': 706, 'pngDecodes': 750, 'verifiedRgbaPixels': 97805184,
                        'structuredAppendHeaders': 135, 'fnc1SecondSymbols': 304, 'damagedSymbolsCorrected': 0,
                        'eciTextModeChecks': 70, 'highLevelSets': 5, 'highLevelSymbols': 44, 'independentReconstructions': 5}
                       if args.decoder == 'cpp' else
                       {'matrixDecodes': 446, 'pngDecodes': 446, 'verifiedRgbaPixels': 10008270 * scale * scale // 9,
                        'structuredAppendHeaders': 135, 'fnc1SecondSymbols': 0, 'damagedSymbolsCorrected': 32,
                        'eciTextModeChecks': 0, 'highLevelSets': 5, 'highLevelSymbols': 44, 'independentReconstructions': 10})
    if counts != expected_counts or len(tests) != (706 if args.decoder == 'cpp' else 402):
        failures.append({'error': 'Strict decoder coverage count mismatch', 'expectedCounts': expected_counts, 'actualCounts': counts})
    end_identity = generate(args.candidate, [{'command': 'identity', 'nonce': candidate_identity['nonce']}])[0]
    if end_identity.get('packageFilesSha256') != candidate_identity.get('packageFilesSha256'):
        failures.append({'error': 'Compiled Go candidate files changed during decoder run'})
    if args.decoder == 'cpp':
        if {str(path): hashlib.sha256(path.read_bytes()).hexdigest() for path in decoder_files} != dependency_hashes:
            failures.append({'error': 'Independent ZXing-C++ dependency changed during decoder run'})
        if node_identity(args.node, ['nayuki-qr-code-generator']) != node_dependencies:
            failures.append({'error': 'Independent Nayuki dependency changed during decoder run'})
    else:
        if hashlib.sha256(jar.read_bytes()).hexdigest() != JAR_SHA256:
            failures.append({'error': 'Independent ZXing Java dependency changed during decoder run'})
        if hashlib.sha256(java_executable.read_bytes()).hexdigest() != java_sha or subprocess.check_output([args.java, '--version'], text=True).splitlines() != java_runtime:
            failures.append({'error': 'Java executable/runtime changed during decoder run'})
    if tool_snapshot() != tool_hashes:
        failures.append({'error': 'Test tools changed during decoder run'})
    if snapshot(args.candidate, 'all') != source_hashes:
        failures.append({'error': 'Candidate sources changed during decoder run'})
    if hashlib.sha256(DRIVER.read_bytes()).hexdigest() != adapter_sha:
        failures.append({'error': 'Adapter changed during decoder run'})
    report = {'status': 'failed' if failures else 'passed', **decoder_identity, 'adapterSha256': adapter_sha, 'sourceSha256': source_hashes, 'testToolSha256': tool_hashes, 'nodeDependencies': node_dependencies, 'binaryConsumer': str(args.binary) if args.binary else None, 'goProcessIdentity': candidate_identity,
              'counts': counts, 'decodedVersions': {k: sorted(v) for k, v in decoded_versions.items()}, 'independentNayukiEciControls': eci_controls, 'strictSymbols': len(tests), 'pngScale': scale, 'pureBarcodeHint': False,
              'defaultScaleDiagnostic': diagnostic_report, 'negativeControls': negative_controls, 'eciApiObservation': ('ZXing-C++ 3.1.1 symbology_identifier is base ]Q1 for ECI; TextMode.ECI independently exposes ]Q2 and transcodes to ECI26 UTF-8. Both are asserted.' if args.decoder == 'cpp' else 'Java returns ]Q2 for ECI; asserted directly.'), 'failures': failures, 'elapsedSeconds': round(time.perf_counter() - started, 3),
              'limitations': ['Synthetic PNG detection is not camera/print certification.', 'Java excludes FNC1-second; ZXing-C++ tests all 152 indicators.', 'Java strict PNG coverage is an isolated scale-3 diagnostic. Default-scale8 detection and its pixel-identical independent control are separately reported and never counted as strict successes.']}
    if corpus_report is not None:
        report['javaDefaultScaleCorpus'] = corpus_report
    report['scopeCompleted'] = True
    _FAILURE_CONTEXT = report
    args.output.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))
    require(not failures, f'{len(failures)} strict independent decoder checks failed')


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        if _FAILURE_OUTPUT is not None:
            _FAILURE_CONTEXT.update(status='failed', terminalError=f'{type(error).__name__}: {error}')
            _FAILURE_OUTPUT.write_text(json.dumps(_FAILURE_CONTEXT, indent=2) + '\n')
        raise
