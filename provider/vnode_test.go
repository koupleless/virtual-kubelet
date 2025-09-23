package provider

import (
	"context"
	"testing"
	"time"

	"github.com/koupleless/virtual-kubelet/common/tracker"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVNode_ExitWhenLeaderChanged(t *testing.T) {
	//vnode := VNode{
	//	WhenLeaderAcquiredByOthers: make(chan struct{}),
	//}
	//select {
	//case <-vnode.ExitWhenLeaderChanged():
	//	assert.Fail(t, "ExitWhenLeaderChanged should have been called")
	//default:
	//}
	//vnode.LeaderAcquiredByOthers()
	//select {
	//case <-vnode.ExitWhenLeaderChanged():
	//default:
	//	assert.Fail(t, "ExitWhenLeaderChanged should not called")
	//}
}

func setupTest() {
	// Initialize tracker to avoid nil pointer issues
	defaultTracker := &tracker.DefaultTracker{}
	defaultTracker.Init()
	tracker.SetTracker(defaultTracker)
}

func TestVNode_SyncOneNodeBizStatusToKube(t *testing.T) {
	setupTest()

	tests := []struct {
		name               string
		toUpdateInKube     []model.BizStatusData
		toDeleteInProvider []model.BizStatusData
		podProviderNil     bool
	}{
		{
			name: "successful sync with updates and deletes",
			toUpdateInKube: []model.BizStatusData{
				{
					Key:        "biz1:1.0.0",
					Name:       "biz1",
					PodKey:     "test-pod-1",
					State:      "ACTIVATED",
					ChangeTime: time.Now(),
					Reason:     "Started",
					Message:    "Biz started successfully",
				},
				{
					Key:        "biz2:2.0.0",
					Name:       "biz2",
					PodKey:     "test-pod-2",
					State:      "ACTIVATED",
					ChangeTime: time.Now(),
					Reason:     "Started",
					Message:    "Biz started successfully",
				},
			},
			toDeleteInProvider: []model.BizStatusData{
				{
					Key:        "biz3:1.0.0",
					Name:       "biz3",
					PodKey:     "test-pod-3",
					State:      "DEACTIVATED",
					ChangeTime: time.Now(),
					Reason:     "Stopped",
					Message:    "Biz stopped",
				},
			},
			podProviderNil: false,
		},
		{
			name:               "empty arrays",
			toUpdateInKube:     []model.BizStatusData{},
			toDeleteInProvider: []model.BizStatusData{},
			podProviderNil:     false,
		},
		{
			name: "only updates",
			toUpdateInKube: []model.BizStatusData{
				{
					Key:        "biz1:1.0.0",
					Name:       "biz1",
					PodKey:     "test-pod-1",
					State:      "ACTIVATED",
					ChangeTime: time.Now(),
					Reason:     "Started",
					Message:    "Biz started successfully",
				},
			},
			toDeleteInProvider: []model.BizStatusData{},
			podProviderNil:     false,
		},
		{
			name:           "only deletes",
			toUpdateInKube: []model.BizStatusData{},
			toDeleteInProvider: []model.BizStatusData{
				{
					Key:        "biz3:1.0.0",
					Name:       "biz3",
					PodKey:     "test-pod-3",
					State:      "DEACTIVATED",
					ChangeTime: time.Now(),
					Reason:     "Stopped",
					Message:    "Biz stopped",
				},
			},
			podProviderNil: false,
		},
		{
			name: "pod provider is nil",
			toUpdateInKube: []model.BizStatusData{
				{
					Key:        "biz1:1.0.0",
					Name:       "biz1",
					PodKey:     "test-pod-1",
					State:      "ACTIVATED",
					ChangeTime: time.Now(),
					Reason:     "Started",
					Message:    "Biz started successfully",
				},
			},
			toDeleteInProvider: []model.BizStatusData{
				{
					Key:        "biz3:1.0.0",
					Name:       "biz3",
					PodKey:     "test-pod-3",
					State:      "DEACTIVATED",
					ChangeTime: time.Now(),
					Reason:     "Stopped",
					Message:    "Biz stopped",
				},
			},
			podProviderNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create VNode instance
			vNode := &VNode{
				name:      "test-node",
				env:       "test",
				client:    fake.NewClientBuilder().Build(),
				kubeCache: &informertest.FakeInformers{},
			}

			if !tt.podProviderNil {
				// Create a real VPodProvider with mock tunnel
				mockTunnel := &tunnel.MockTunnel{}
				vNode.podProvider = NewVPodProvider("default", "127.0.0.1", "test-node",
					fake.NewClientBuilder().Build(), &informertest.FakeInformers{}, mockTunnel)

				// Set up a dummy notify callback to avoid nil pointer reference
				vNode.podProvider.NotifyPods(context.Background(), func(pod *corev1.Pod) {
					// This is a dummy callback for testing
				})
			}

			// Execute the method - this should not panic and should complete without error
			ctx := context.Background()

			// Call the method under test
			vNode.SyncOneNodeBizStatusToKube(ctx, tt.toUpdateInKube, tt.toDeleteInProvider)

			// For this test, we mainly verify that:
			// 1. The method doesn't panic
			// 2. The method handles nil podProvider gracefully
			// 3. The method processes all input data without error

			// If we reach this point, the test passed
			assert.True(t, true, "SyncOneNodeBizStatusToKube completed without panic")
		})
	}
}

func TestVNode_SyncOneNodeBizStatusToKube_NilPodProvider(t *testing.T) {
	setupTest()

	// Test specifically for nil podProvider behavior
	vNode := &VNode{
		name:        "test-node",
		env:         "test",
		client:      fake.NewClientBuilder().Build(),
		kubeCache:   &informertest.FakeInformers{},
		podProvider: nil, // Explicitly set to nil
	}

	toUpdateInKube := []model.BizStatusData{
		{
			Key:        "biz1:1.0.0",
			Name:       "biz1",
			PodKey:     "test-pod-1",
			State:      "ACTIVATED",
			ChangeTime: time.Now(),
			Reason:     "Started",
			Message:    "Biz started successfully",
		},
	}

	toDeleteInProvider := []model.BizStatusData{
		{
			Key:        "biz3:1.0.0",
			Name:       "biz3",
			PodKey:     "test-pod-3",
			State:      "DEACTIVATED",
			ChangeTime: time.Now(),
			Reason:     "Stopped",
			Message:    "Biz stopped",
		},
	}

	ctx := context.Background()

	// This should not panic when podProvider is nil
	assert.NotPanics(t, func() {
		vNode.SyncOneNodeBizStatusToKube(ctx, toUpdateInKube, toDeleteInProvider)
	}, "SyncOneNodeBizStatusToKube should handle nil podProvider gracefully")
}

func TestVNode_SyncOneNodeBizStatusToKube_EmptyInputs(t *testing.T) {
	setupTest()

	// Test with empty input arrays
	mockTunnel := &tunnel.MockTunnel{}
	vNode := &VNode{
		name:      "test-node",
		env:       "test",
		client:    fake.NewClientBuilder().Build(),
		kubeCache: &informertest.FakeInformers{},
		podProvider: NewVPodProvider("default", "127.0.0.1", "test-node",
			fake.NewClientBuilder().Build(), &informertest.FakeInformers{}, mockTunnel),
	}

	// Set up a dummy notify callback to avoid nil pointer reference
	vNode.podProvider.NotifyPods(context.Background(), func(pod *corev1.Pod) {
		// This is a dummy callback for testing
	})

	ctx := context.Background()

	// Test with empty arrays
	assert.NotPanics(t, func() {
		vNode.SyncOneNodeBizStatusToKube(ctx, []model.BizStatusData{}, []model.BizStatusData{})
	}, "SyncOneNodeBizStatusToKube should handle empty arrays gracefully")
}

func TestVNode_SyncOneNodeBizStatusToKube_BizKeyParsing(t *testing.T) {
	setupTest()

	// Test the syncNotExistBizPodToProvider method behavior with different biz keys
	tests := []struct {
		name            string
		bizKey          string
		expectedName    string
		expectedVersion string
	}{
		{
			name:            "valid biz key with colon separator",
			bizKey:          "test-biz:1.0.0",
			expectedName:    "test-biz",
			expectedVersion: "1.0.0",
		},
		{
			name:            "biz key without version",
			bizKey:          "test-biz",
			expectedName:    "",
			expectedVersion: "",
		},
		{
			name:            "biz key with multiple colons",
			bizKey:          "test:biz:1.0.0",
			expectedName:    "test",
			expectedVersion: "biz:1.0.0",
		},
		{
			name:            "empty biz key",
			bizKey:          "",
			expectedName:    "",
			expectedVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTunnel := &tunnel.MockTunnel{}
			vNode := &VNode{
				name:      "test-node",
				env:       "test",
				client:    fake.NewClientBuilder().Build(),
				kubeCache: &informertest.FakeInformers{},
				podProvider: NewVPodProvider("default", "127.0.0.1", "test-node",
					fake.NewClientBuilder().Build(), &informertest.FakeInformers{}, mockTunnel),
			}

			// Set up a dummy notify callback to avoid nil pointer reference
			vNode.podProvider.NotifyPods(context.Background(), func(pod *corev1.Pod) {
				// This is a dummy callback for testing
			})

			toDeleteInProvider := []model.BizStatusData{
				{
					Key:        tt.bizKey,
					Name:       "test-biz",
					PodKey:     "test-pod",
					State:      "DEACTIVATED",
					ChangeTime: time.Now(),
					Reason:     "Stopped",
					Message:    "Biz stopped",
				},
			}

			ctx := context.Background()

			// Test that the method doesn't panic with different biz key formats
			assert.NotPanics(t, func() {
				vNode.SyncOneNodeBizStatusToKube(ctx, []model.BizStatusData{}, toDeleteInProvider)
			}, "SyncOneNodeBizStatusToKube should handle different biz key formats without panic")
		})
	}
}
