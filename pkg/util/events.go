/*
Copyright (c) Microsoft Corporation.
Licensed under the Apache 2.0 license.
*/
package util

import (
	"context"

	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
)

func NewRecorder(ctx context.Context, kubeClient *kubernetes.Clientset) record.EventBroadcaster {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(3)
	if eventBroadcaster != nil && kubeClient != nil {
		eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	}
	return eventBroadcaster
}
