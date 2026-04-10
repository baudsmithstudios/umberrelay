package web

import (
	"os"
	"strings"
	"testing"
)

func TestChartsJSAvoidsRetroEffects(t *testing.T) {
	script, err := os.ReadFile("static/js/charts.js")
	if err != nil {
		t.Fatalf("ReadFile(static/js/charts.js): %v", err)
	}
	source := string(script)

	if strings.Contains(source, "Scanline overlay") {
		t.Fatalf("charts.js should not include retro scanline effects")
	}
	if strings.Contains(source, "ctx.shadowBlur = 6") {
		t.Fatalf("charts.js should not include glow blur effects")
	}
	if strings.Contains(source, "drawRetroChart(") {
		t.Fatalf("charts.js should no longer use the retro chart renderer")
	}
	if !strings.Contains(source, "function drawChart(canvas, datasets, xlabels)") {
		t.Fatalf("charts.js should expose a clear chart renderer entrypoint")
	}
}

func TestChartsJSSnapsTextAndCanvasToPixels(t *testing.T) {
	script, err := os.ReadFile("static/js/charts.js")
	if err != nil {
		t.Fatalf("ReadFile(static/js/charts.js): %v", err)
	}
	source := string(script)

	if !strings.Contains(source, "function snapPixel(value, dpr)") {
		t.Fatalf("charts.js should provide pixel snapping for axis labels")
	}
	if !strings.Contains(source, "Math.round(rect.width * dpr)") {
		t.Fatalf("charts.js should round canvas backing width to whole pixels")
	}
	if !strings.Contains(source, "Math.round(rect.height * dpr)") {
		t.Fatalf("charts.js should round canvas backing height to whole pixels")
	}
}

func TestChartsJSUsesRightAxisForTrackerValues(t *testing.T) {
	script, err := os.ReadFile("static/js/charts.js")
	if err != nil {
		t.Fatalf("ReadFile(static/js/charts.js): %v", err)
	}
	source := string(script)

	if !strings.Contains(source, "var secondaryVals = datasets[1] ? datasets[1].values : [];") {
		t.Fatalf("charts.js should derive right-axis scale from tracker dataset")
	}
	if !strings.Contains(source, "ctx.fillText(rightLabel.toString(), pad.left + plotW + 6") {
		t.Fatalf("charts.js should render right-axis value labels")
	}
	if !strings.Contains(source, "axis: 'right'") {
		t.Fatalf("charts.js should mark tracker series for right-axis scaling")
	}
}
