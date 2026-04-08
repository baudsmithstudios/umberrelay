package web

import (
	"os"
	"strings"
	"testing"
)

func TestChartsJSUsesSmoothLinePath(t *testing.T) {
	script, err := os.ReadFile("static/js/charts.js")
	if err != nil {
		t.Fatalf("ReadFile(static/js/charts.js): %v", err)
	}
	source := string(script)

	if strings.Contains(source, "ctx.lineTo(x, lastY);") {
		t.Fatalf("charts.js still contains step-style horizontal segment drawing")
	}
	if !strings.Contains(source, "ctx.bezierCurveTo(") {
		t.Fatalf("charts.js should draw smooth curves using bezier segments")
	}
}
