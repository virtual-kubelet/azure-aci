// Code generated by MockGen. DO NOT EDIT.
// Source: .\metrics.go

// Package mock_metrics is a generated GoMock package.
package metrics

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	"github.com/virtual-kubelet/azure-aci/pkg/client"
	statsv1alpha1 "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	v1 "k8s.io/api/core/v1"
)

// MockPodGetter is a mock of PodGetter interface.
type MockPodGetter struct {
	ctrl     *gomock.Controller
	recorder *MockPodGetterMockRecorder
}

// MockPodGetterMockRecorder is the mock recorder for MockPodGetter.
type MockPodGetterMockRecorder struct {
	mock *MockPodGetter
}

// NewMockPodGetter creates a new mock instance.
func NewMockPodGetter(ctrl *gomock.Controller) *MockPodGetter {
	mock := &MockPodGetter{ctrl: ctrl}
	mock.recorder = &MockPodGetterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPodGetter) EXPECT() *MockPodGetterMockRecorder {
	return m.recorder
}

// GetPods mocks base method.
func (m *MockPodGetter) GetPods() []*v1.Pod {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPods")
	ret0, _ := ret[0].([]*v1.Pod)
	return ret0
}

// GetPods indicates an expected call of GetPods.
func (mr *MockPodGetterMockRecorder) GetPods() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPods", reflect.TypeOf((*MockPodGetter)(nil).GetPods))
}

// MockMetricsGetter is a mock of MetricsGetter interface.
type MockMetricsGetter struct {
	ctrl     *gomock.Controller
	recorder *MockMetricsGetterMockRecorder
}

// MockMetricsGetterMockRecorder is the mock recorder for MockMetricsGetter.
type MockMetricsGetterMockRecorder struct {
	mock *MockMetricsGetter
}

// NewMockMetricsGetter creates a new mock instance.
func NewMockMetricsGetter(ctrl *gomock.Controller) *MockMetricsGetter {
	mock := &MockMetricsGetter{ctrl: ctrl}
	mock.recorder = &MockMetricsGetterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockMetricsGetter) EXPECT() *MockMetricsGetterMockRecorder {
	return m.recorder
}

// MockContainerGroupGetter is a mock of ContainerGroupGetter interface.
type MockContainerGroupGetter struct {
	ctrl     *gomock.Controller
	recorder *MockContainerGroupGetterMockRecorder
}

// MockContainerGroupGetterMockRecorder is the mock recorder for MockContainerGroupGetter.
type MockContainerGroupGetterMockRecorder struct {
	mock *MockContainerGroupGetter
}

// NewMockContainerGroupGetter creates a new mock instance.
func NewMockContainerGroupGetter(ctrl *gomock.Controller) *MockContainerGroupGetter {
	mock := &MockContainerGroupGetter{ctrl: ctrl}
	mock.recorder = &MockContainerGroupGetterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockContainerGroupGetter) EXPECT() *MockContainerGroupGetterMockRecorder {
	return m.recorder
}

// GetContainerGroup mocks base method.
func (m *MockContainerGroupGetter) GetContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) (*client.ContainerGroupWrapper, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetContainerGroupInfo", ctx, resourceGroup, containerGroupName)
	ret0, _ := ret[0].(*client.ContainerGroupWrapper)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetContainerGroup indicates an expected call of GetContainerGroup.
func (mr *MockContainerGroupGetterMockRecorder) GetContainerGroup(ctx, resourceGroup, containerGroupName interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetContainerGroupInfo", reflect.TypeOf((*MockContainerGroupGetter)(nil).GetContainerGroup), ctx, resourceGroup, containerGroupName)
}

// MockpodStatsGetter is a mock of podStatsGetter interface.
type MockpodStatsGetter struct {
	ctrl     *gomock.Controller
	recorder *MockpodStatsGetterMockRecorder
}

// MockpodStatsGetterMockRecorder is the mock recorder for MockpodStatsGetter.
type MockpodStatsGetterMockRecorder struct {
	mock *MockpodStatsGetter
}

// NewMockpodStatsGetter creates a new mock instance.
func NewMockpodStatsGetter(ctrl *gomock.Controller) *MockpodStatsGetter {
	mock := &MockpodStatsGetter{ctrl: ctrl}
	mock.recorder = &MockpodStatsGetterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockpodStatsGetter) EXPECT() *MockpodStatsGetterMockRecorder {
	return m.recorder
}

// getPodStats mocks base method.
func (m *MockpodStatsGetter) GetPodStats(ctx context.Context, pod *v1.Pod) (*statsv1alpha1.PodStats, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPodStats", ctx, pod)
	ret0, _ := ret[0].(*statsv1alpha1.PodStats)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// getPodStats indicates an expected call of getPodStats.
func (mr *MockpodStatsGetterMockRecorder) GetPodStats(ctx, pod interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPodStats", reflect.TypeOf((*MockpodStatsGetter)(nil).GetPodStats), ctx, pod)
}

// getContainerGroupFromPod mocks base method.
func (m *MockpodStatsGetter) getContainerGroupFromPod(ctx context.Context, pod *v1.Pod) (*client.ContainerGroupWrapper, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "getContainerGroupFromPod", ctx, pod)
	ret0, _ := ret[0].(*client.ContainerGroupWrapper)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// getPodStats indicates an expected call of getPodStats.
func (mr *MockpodStatsGetterMockRecorder) getContainerGroupFromPod(ctx, pod interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "getContainerGroupFromPod", reflect.TypeOf((*MockpodStatsGetter)(nil).getContainerGroupFromPod), ctx, pod)
}
