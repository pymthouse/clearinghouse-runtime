package admin

import "context"

type OpenMeterAdmin interface {
	WaitForHealthy(ctx context.Context) error
	ListMeters(ctx context.Context) ([]Meter, error)
	CreateMeter(ctx context.Context, input MeterInput) (*Meter, error)
	DeleteMeter(ctx context.Context, id string) error
	ListFeatures(ctx context.Context) ([]Feature, error)
	CreateFeature(ctx context.Context, input FeatureInput) (*Feature, error)
	DeleteFeature(ctx context.Context, id string) error
	ListPlans(ctx context.Context) ([]Plan, error)
	EnsurePlan(ctx context.Context, input PlanInput) (*Plan, error)
	DeletePlan(ctx context.Context, id string) error
}
