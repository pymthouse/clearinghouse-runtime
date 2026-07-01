package openmeter

import "context"

// ProvisionSession upserts customer, default subscription, trial grant, and returns allowance.
func (c *Client) ProvisionSession(ctx context.Context, cfg ProvisionConfig, clientID, externalUserID string) (*SessionProvision, error) {
	customerKey := CustomerKey(clientID, externalUserID)

	customer, err := c.EnsureCustomer(ctx, clientID, externalUserID, externalUserID)
	if err != nil {
		return nil, err
	}

	if err := c.EnsureDefaultSubscription(ctx, customer.ID, customerKey, cfg.DefaultPlanKey); err != nil {
		return nil, err
	}

	if err := c.EnsureTrialGrant(ctx, customerKey, cfg.TrialFeatureKey, cfg.DefaultStarterIncludedMicros); err != nil {
		// Konnect may not support explicit grants; continue with subscription-only allowance.
		_ = err
	}

	balance, err := c.GetTrialCreditBalanceWithFallback(ctx, customerKey, cfg.TrialFeatureKey, cfg.DefaultStarterIncludedMicros)
	if err != nil {
		return nil, err
	}

	return &SessionProvision{
		Customer:    customer,
		CustomerKey: customerKey,
		Balance:     balance,
	}, nil
}
