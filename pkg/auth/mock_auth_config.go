// Code generated by MockGen. DO NOT EDIT.
// Source: auth_config.go

// Package auth is a generated GoMock package.
package auth

import (
	context "context"
	reflect "reflect"

	azidentity "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	autorest "github.com/Azure/go-autorest/autorest"
	gomock "github.com/golang/mock/gomock"
)

// MockConfigInterface is a mock of ConfigInterface interface.
type MockConfigInterface struct {
	ctrl     *gomock.Controller
	recorder *MockConfigInterfaceMockRecorder
}

// MockConfigInterfaceMockRecorder is the mock recorder for MockConfigInterface.
type MockConfigInterfaceMockRecorder struct {
	mock *MockConfigInterface
}

// NewMockConfigInterface creates a new mock instance.
func NewMockConfigInterface(ctrl *gomock.Controller) *MockConfigInterface {
	mock := &MockConfigInterface{ctrl: ctrl}
	mock.recorder = &MockConfigInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConfigInterface) EXPECT() *MockConfigInterfaceMockRecorder {
	return m.recorder
}

// GetAuthorizer mocks base method.
func (m *MockConfigInterface) GetAuthorizer(ctx context.Context, resource string) (autorest.Authorizer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAuthorizer", ctx, resource)
	ret0, _ := ret[0].(autorest.Authorizer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetAuthorizer indicates an expected call of GetAuthorizer.
func (mr *MockConfigInterfaceMockRecorder) GetAuthorizer(ctx, resource interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAuthorizer", reflect.TypeOf((*MockConfigInterface)(nil).GetAuthorizer), ctx, resource)
}

// GetMSICredential mocks base method.
func (m *MockConfigInterface) GetMSICredential(ctx context.Context) (*azidentity.ManagedIdentityCredential, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetMSICredential", ctx)
	ret0, _ := ret[0].(*azidentity.ManagedIdentityCredential)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetMSICredential indicates an expected call of GetMSICredential.
func (mr *MockConfigInterfaceMockRecorder) GetMSICredential(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetMSICredential", reflect.TypeOf((*MockConfigInterface)(nil).GetMSICredential), ctx)
}

// GetSPCredential mocks base method.
func (m *MockConfigInterface) GetSPCredential(ctx context.Context) (*azidentity.ClientSecretCredential, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetSPCredential", ctx)
	ret0, _ := ret[0].(*azidentity.ClientSecretCredential)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetSPCredential indicates an expected call of GetSPCredential.
func (mr *MockConfigInterfaceMockRecorder) GetSPCredential(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetSPCredential", reflect.TypeOf((*MockConfigInterface)(nil).GetSPCredential), ctx)
}
