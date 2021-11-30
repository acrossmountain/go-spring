// Code generated by MockGen. DO NOT EDIT.
// Source: assert/assert.go

// Package assert is a generated GoMock package.
package assert

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockT is a mock of T interface.
type MockT struct {
	ctrl     *gomock.Controller
	recorder *MockTMockRecorder
}

// MockTMockRecorder is the mock recorder for MockT.
type MockTMockRecorder struct {
	mock *MockT
}

// NewMockT creates a new mock instance.
func NewMockT(ctrl *gomock.Controller) *MockT {
	mock := &MockT{ctrl: ctrl}
	mock.recorder = &MockTMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockT) EXPECT() *MockTMockRecorder {
	return m.recorder
}

// Fail mocks base method.
func (m *MockT) Fail() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Fail")
}

// Fail indicates an expected call of Fail.
func (mr *MockTMockRecorder) Fail() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Fail", reflect.TypeOf((*MockT)(nil).Fail))
}

// Helper mocks base method.
func (m *MockT) Helper() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Helper")
}

// Helper indicates an expected call of Helper.
func (mr *MockTMockRecorder) Helper() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Helper", reflect.TypeOf((*MockT)(nil).Helper))
}

// Log mocks base method.
func (m *MockT) Log(args ...interface{}) {
	m.ctrl.T.Helper()
	varargs := []interface{}{}
	for _, a := range args {
		varargs = append(varargs, a)
	}
	m.ctrl.Call(m, "Log", varargs...)
}

// Log indicates an expected call of Log.
func (mr *MockTMockRecorder) Log(args ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Log", reflect.TypeOf((*MockT)(nil).Log), args...)
}
