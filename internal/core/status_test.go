package core

import "testing"

func TestNoopStatus_UpdateStatus(_ *testing.T) {
	s := NoopStatus{}
	s.UpdateStatus("anything")
}

func TestUpdateGenerateStatus_NilOpts(_ *testing.T) {
	UpdateGenerateStatus(nil, "msg")
}

func TestUpdateGenerateStatus_NilStatus(_ *testing.T) {
	opts := &GenerateOptions{}
	UpdateGenerateStatus(opts, "msg")
}

func TestUpdateGenerateStatus_Delivers(t *testing.T) {
	var got string
	updater := &funcStatus{fn: func(msg string) { got = msg }}
	opts := &GenerateOptions{Status: updater}

	UpdateGenerateStatus(opts, "hello")

	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

type funcStatus struct {
	fn func(string)
}

func (f *funcStatus) UpdateStatus(msg string) { f.fn(msg) }
