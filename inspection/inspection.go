package inspection

import (
	"context"
	"k8s.io/client-go/kubernetes"
	"time"
)

var RegisteredInspection = []Inspection{
	&PodScheduleInspection{},
}

type Inspection interface {
	// Register is the init func of inspections
	Register(kubernetes.Interface)

	// GetIntervalMilliSec returns the inspect interval
	GetInterval() time.Duration

	// Inspect is the main check of inspections
	Inspect(context.Context)
}
