package web

import (
	"os"
	"strings"
	"testing"
)

func ruleBlock(css, selector string) string {
	start := strings.Index(css, selector+" {")
	if start < 0 {
		return ""
	}
	rest := css[start:]
	end := strings.Index(rest, "}\n")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func TestThemeCSSCardTopLinesUseCalibratedVerticalShift(t *testing.T) {
	cssBytes, err := os.ReadFile("static/css/theme.css")
	if err != nil {
		t.Fatalf("ReadFile(static/css/theme.css): %v", err)
	}
	css := string(cssBytes)

	sectionTitle := ruleBlock(css, ".section-title")
	if sectionTitle == "" {
		t.Fatalf("theme.css missing .section-title selector")
	}
	if strings.Contains(sectionTitle, "transform: translateY(-50%);") {
		t.Fatalf("theme.css should avoid aggressive .section-title vertical shift")
	}
	if !strings.Contains(sectionTitle, "transform: translateY(-45%);") {
		t.Fatalf("theme.css should apply a calibrated .section-title vertical shift")
	}

	chartHeader := ruleBlock(css, ".chart-header")
	if chartHeader == "" {
		t.Fatalf("theme.css missing .chart-header selector")
	}
	if strings.Contains(chartHeader, "transform: translateY(-50%);") {
		t.Fatalf("theme.css should avoid aggressive .chart-header vertical shift")
	}
	if !strings.Contains(chartHeader, "transform: translateY(-45%);") {
		t.Fatalf("theme.css should apply a calibrated .chart-header vertical shift")
	}
}

func TestThemeCSSTitleMarginsUseCardPaddingVariables(t *testing.T) {
	cssBytes, err := os.ReadFile("static/css/theme.css")
	if err != nil {
		t.Fatalf("ReadFile(static/css/theme.css): %v", err)
	}
	css := string(cssBytes)

	sectionTitle := ruleBlock(css, ".section-title")
	if !strings.Contains(sectionTitle, "var(--umberrelay-card-pad-y)") {
		t.Fatalf(".section-title margin should use --umberrelay-card-pad-y")
	}
	if !strings.Contains(sectionTitle, "var(--umberrelay-card-pad-x)") {
		t.Fatalf(".section-title margin should use --umberrelay-card-pad-x")
	}

	chartHeader := ruleBlock(css, ".chart-header")
	if !strings.Contains(chartHeader, "var(--umberrelay-card-pad-y)") {
		t.Fatalf(".chart-header margin should use --umberrelay-card-pad-y")
	}
	if !strings.Contains(chartHeader, "var(--umberrelay-card-pad-x)") {
		t.Fatalf(".chart-header margin should use --umberrelay-card-pad-x")
	}
}

func TestThemeCSSTrailingTitleLineExtendsRightEdge(t *testing.T) {
	cssBytes, err := os.ReadFile("static/css/theme.css")
	if err != nil {
		t.Fatalf("ReadFile(static/css/theme.css): %v", err)
	}
	css := string(cssBytes)

	sectionAfter := ruleBlock(css, ".section-title::after")
	if strings.Contains(sectionAfter, "margin-right: -1px;") {
		t.Fatalf(".section-title::after should not rely on margin-right for right-edge alignment")
	}
	if !strings.Contains(sectionAfter, "position: relative;") || !strings.Contains(sectionAfter, "\n    right: -1px;") {
		t.Fatalf(".section-title::after should shift 1px to the right")
	}

	chartAfter := ruleBlock(css, ".chart-header::after")
	if strings.Contains(chartAfter, "margin-right: -1px;") {
		t.Fatalf(".chart-header::after should not rely on margin-right for right-edge alignment")
	}
	if !strings.Contains(chartAfter, "position: relative;") || !strings.Contains(chartAfter, "\n    right: -1px;") {
		t.Fatalf(".chart-header::after should shift 1px to the right")
	}
}
