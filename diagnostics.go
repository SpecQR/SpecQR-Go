package specqr

func makeDiagnostics(ss []Segment, v int, ecc ECC, bits int, o Options, planning, ok bool) map[string]any {
	n, _ := DataCodewordCount(v, ecc)
	capacity := n * 8
	controls := []any{}
	segments := []any{}
	modes := []Mode{}
	inputBytes := 0
	var appendHeader, second, eci *Segment
	gs1 := false
	for i, s := range ss {
		b, _ := s.BitLength(v)
		segments = append(segments, map[string]any{"mode": string(s.Mode()), "characterCount": s.CharacterCount(), "byteCount": s.ByteCount(), "bitLength": b})
		inputBytes += len(s.LogicalBytes())
		if s.IsControl() {
			controls = append(controls, map[string]any{"mode": string(s.Mode()), "bitLength": b})
		} else {
			found := false
			for _, m := range modes {
				if m == s.Mode() {
					found = true
				}
			}
			if !found {
				modes = append(modes, s.Mode())
			}
		}
		switch s.Mode() {
		case StructuredAppend:
			appendHeader = &ss[i]
		case FNC1Second:
			second = &ss[i]
		case ECI:
			eci = &ss[i]
		case FNC1First:
			gs1 = true
		}
	}
	mode := "byte"
	if len(modes) == 1 {
		mode = string(modes[0])
	} else if len(modes) > 1 {
		mode = "mixed"
	}
	warnings := []any{}
	warn := func(code, severity, message string, details map[string]any) {
		warnings = append(warnings, map[string]any{"code": code, "severity": severity, "message": message, "details": details})
	}
	if o.Render.Margin < 4 {
		warn("QUIET_ZONE_TOO_SMALL", "warning", "QR readers expect at least four quiet-zone modules.", map[string]any{"margin": o.Render.Margin})
	}
	fg, fe := ParseColor(o.Render.Foreground)
	bg, be := ParseColor(o.Render.Background)
	inspect := fe == nil && be == nil
	var ratio, fa, ba any
	strong, sufficient := false, false
	if inspect {
		r := ContrastRatio(fg, bg)
		ratio = r
		fa = int(fg[3])
		ba = int(bg[3])
		strong = r >= 7
		sufficient = r >= 4.5 && fg[3] == 255 && bg[3] == 255
		if r < 4.5 {
			warn("COLOR_CONTRAST_LOW", "warning", "Color contrast is below the recommended minimum.", map[string]any{"ratio": r})
		} else if r < 7 {
			warn("COLOR_CONTRAST_MODERATE", "info", "Stronger contrast is recommended.", map[string]any{"ratio": r})
		}
		if fg[3] < 255 || bg[3] < 255 {
			warn("COLOR_ALPHA_USED", "warning", "Transparency can reduce scan reliability.", map[string]any{})
		}
	} else {
		warn("COLOR_CONTRAST_UNKNOWN", "info", "These SVG colors cannot be checked for contrast.", map[string]any{})
	}
	if capacity >= bits && float64(capacity-bits) < float64(capacity)*.05 {
		warn("CAPACITY_NEAR_LIMIT", "info", "Selected version is close to full.", map[string]any{})
	}
	var dpi, mm, symbolMM, moduleSufficient any
	if o.PrintDPI > 0 {
		dpi = o.PrintDPI
		m := float64(o.Render.Scale) / o.PrintDPI * 25.4
		mm = m
		symbolMM = (float64(v*4+17) + 2*float64(o.Render.Margin)) * m
		moduleSufficient = m >= .25
		if m < .25 {
			warn("PRINT_MODULE_TOO_SMALL", "warning", "Print modules are smaller than 0.25 mm.", map[string]any{"moduleSizeMm": m})
		}
	}
	blocking := []any{}
	for _, w := range warnings {
		m := w.(map[string]any)
		if m["severity"] == "warning" {
			blocking = append(blocking, m["code"])
		}
	}
	if len(blocking) > 0 {
		warn("SCAN_RISK", "warning", "One or more settings may reduce scan reliability.", map[string]any{"blockingWarnings": blocking})
	}
	phase := "generation"
	if planning {
		phase = "planning"
	}
	selection, reason := "auto-range", "No fitting version in the requested range."
	if o.Version > 0 {
		selection = "fixed"
		reason = "Explicit version requested."
	} else if ok {
		selection = "auto-minimum"
		reason = "Smallest fitting version in the requested range."
	}
	var visibleVersion, size any
	if ok || o.Version > 0 {
		visibleVersion = v
		size = v*4 + 17
	}
	var fnc1, assignment, indicator, indicatorCode, index, total, parity, sequenceIndex, sequenceTotal, sequenceIndicator any
	if gs1 {
		fnc1 = "first-position"
	} else if second != nil {
		fnc1 = "second-position"
	}
	if eci != nil {
		assignment = eci.ECIAssignment()
	}
	if second != nil {
		indicator = second.ApplicationIndicator()
		indicatorCode = second.ApplicationIndicatorCodeword()
	}
	if appendHeader != nil {
		i, t := appendHeader.Index(), appendHeader.Total()
		index = i
		total = t
		parity = appendHeader.Parity()
		sequenceIndex = i - 1
		sequenceTotal = t - 1
		sequenceIndicator = ((i - 1) << 4) | (t - 1)
	}
	var elementCount any = 0
	if gs1 {
		elementCount = nil
	}
	return map[string]any{
		"phase": phase, "renderPlanned": false, "maskEvaluated": !planning, "codewordsBuilt": !planning, "version": visibleVersion, "size": size, "errorCorrectionLevel": string(ecc), "requestedErrorCorrectionLevel": string(o.ECC), "boostedErrorCorrection": ecc != o.ECC, "versionSelection": selection, "versionSelectionReason": reason, "mode": mode, "controlSegments": controls, "eciAssignmentNumber": assignment, "fnc1": fnc1, "gs1": gs1,
		"gs1Validation":    map[string]any{"enabled": gs1, "elementCount": elementCount, "ais": []any{}, "hasSeparators": false},
		"fnc1Second":       map[string]any{"enabled": second != nil, "applicationIndicator": indicator, "applicationIndicatorCodeword": indicatorCode},
		"structuredAppend": map[string]any{"enabled": appendHeader != nil, "index": index, "total": total, "parity": parity, "sequenceIndex": sequenceIndex, "sequenceTotal": sequenceTotal, "sequenceIndicator": sequenceIndicator},
		"segments":         segments, "dataBitLength": bits, "capacityBits": capacity, "remainingBits": capacity - bits, "capacityUtilization": float64(bits) / float64(capacity), "inputBytes": inputBytes,
		"quietZone": map[string]any{"modules": o.Render.Margin, "recommendedModules": 4, "isSufficient": o.Render.Margin >= 4},
		"colors":    map[string]any{"ratio": ratio, "isInspectable": inspect, "foregroundAlpha": fa, "backgroundAlpha": ba, "isStrong": strong, "isSufficient": sufficient},
		"print":     map[string]any{"dpi": dpi, "modulePixels": o.Render.Scale, "moduleSizeMm": mm, "symbolSizeMm": symbolMM, "recommendedMinimumModuleSizeMm": .25, "isModuleSizeSufficient": moduleSufficient}, "warnings": warnings,
	}
}
