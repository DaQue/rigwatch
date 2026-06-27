package ui

import (
	"testing"
	"time"

	"github.com/allisonhere/rigwatch/internal"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAnimationTickIncrementsFrame(t *testing.T) {
	m := Model{animationFrame: 41}

	updated, cmd := m.Update(AnimationTickMsg(time.Now()))
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want ui.Model", updated)
	}
	if got.animationFrame != 42 {
		t.Fatalf("animationFrame = %d, want 42", got.animationFrame)
	}
	if cmd == nil {
		t.Fatalf("expected animation tick command to be rescheduled")
	}
}

func TestAnimationTickCommandEmitsMessage(t *testing.T) {
	msg := animationTick()()
	if _, ok := msg.(AnimationTickMsg); !ok {
		t.Fatalf("animationTick emitted %T, want AnimationTickMsg", msg)
	}
}

func TestInitialModelStartsAnimation(t *testing.T) {
	m := InitialModel(nil, time.Second)
	cmd := m.Init()
	if cmd == nil {
		t.Fatalf("expected Init to return batched commands")
	}
	if _, ok := interface{}(cmd).(tea.Cmd); !ok {
		t.Fatalf("expected tea.Cmd")
	}
}

func TestAppendMetricHistoryClampsPerHostSamples(t *testing.T) {
	m := InitialModel(nil, time.Second)
	for i := 0; i < metricHistoryLimit+5; i++ {
		m.appendMetricHistory("host-a", &internal.SystemInfo{CPU: internal.CPUInfo{UsagePercent: float64(i)}})
	}

	history := m.metricHistories["host-a"]
	if len(history.CPU) != metricHistoryLimit {
		t.Fatalf("CPU history len = %d, want %d", len(history.CPU), metricHistoryLimit)
	}
	if history.CPU[0] != 5 {
		t.Fatalf("oldest retained CPU sample = %.1f, want 5.0", history.CPU[0])
	}
}
