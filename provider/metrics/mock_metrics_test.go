// Code generated by MockGen. DO NOT EDIT.
// Source: .\metrics.go

// Package mock_metrics is a generated GoMock package.
package metrics

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	aci "github.com/virtual-kubelet/azure-aci/client/aci"
	statsv1alpha1 "github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1listers "k8s.io/client-go/listers/core/v1"
)

// MockPodLister is a mock of PodLister interface.
type MockPodLister struct {
	ctrl     *gomock.Controller
	recorder *MockPodListerMockRecorder
}

func (m *MockPodLister) List(selector labels.Selector) (ret []*v1.Pod, err error) {
	m.ctrl.T.Helper()
	mList := m.ctrl.Call(m, "List", selector)
	mList0 := mList[0].([]*v1.Pod)
	mList1, _ := mList[1].(error)
	return mList0, mList1
}

func (m *MockPodLister) Pods(namespace string) corev1listers.PodNamespaceLister {
	return nil
}

func (m *MockPodListerMockRecorder) List(selector interface{}) *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(m.mock, "List", reflect.TypeOf((*MockPodLister)(nil).List), selector)
}

// MockPodListerMockRecorder is the mock recorder for MockPodLister.
type MockPodListerMockRecorder struct {
	mock *MockPodLister
}

// NewMockPodLister creates a new mock instance.
func NewMockPodLister(ctrl *gomock.Controller) *MockPodLister {
	mock := &MockPodLister{ctrl: ctrl}
	mock.recorder = &MockPodListerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPodLister) EXPECT() *MockPodListerMockRecorder {
	return m.recorder
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

// GetContainerGroupMetrics mocks base method.
func (m *MockMetricsGetter) GetContainerGroupMetrics(ctx context.Context, resourceGroup, containerGroup string, options aci.MetricsRequest) (*aci.ContainerGroupMetricsResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetContainerGroupMetrics", ctx, resourceGroup, containerGroup, options)
	ret0, _ := ret[0].(*aci.ContainerGroupMetricsResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetContainerGroupMetrics indicates an expected call of GetContainerGroupMetrics.
func (mr *MockMetricsGetterMockRecorder) GetContainerGroupMetrics(ctx, resourceGroup, containerGroup, options interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetContainerGroupMetrics", reflect.TypeOf((*MockMetricsGetter)(nil).GetContainerGroupMetrics), ctx, resourceGroup, containerGroup, options)
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
func (m *MockContainerGroupGetter) GetContainerGroup(ctx context.Context, resourceGroup, containerGroupName string) (*aci.ContainerGroup, *int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetContainerGroup", ctx, resourceGroup, containerGroupName)
	ret0, _ := ret[0].(*aci.ContainerGroup)
	ret1, _ := ret[1].(*int)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetContainerGroup indicates an expected call of GetContainerGroup.
func (mr *MockContainerGroupGetterMockRecorder) GetContainerGroup(ctx, resourceGroup, containerGroupName interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetContainerGroup", reflect.TypeOf((*MockContainerGroupGetter)(nil).GetContainerGroup), ctx, resourceGroup, containerGroupName)
}

// MockpodStatsGetter is a mock of PodStatsGetter interface.
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

// GetPodStats indicates an expected call of GetPodStats.
func (mr *MockpodStatsGetterMockRecorder) GetPodStats(ctx, pod interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPodStats", reflect.TypeOf((*MockpodStatsGetter)(nil).GetPodStats), ctx, pod)
}
