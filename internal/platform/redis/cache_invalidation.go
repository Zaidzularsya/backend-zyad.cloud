package redis

import (
	"context"

	corecache "zyad.cloud/internal/core/cache"
)

func (c *Client) PublishCacheInvalidation(
	ctx context.Context,
	event corecache.InvalidationEvent,
) error {
	payload, err := event.MarshalJSONPayload()
	if err != nil {
		return err
	}
	if c == nil || c.Client == nil {
		return nil
	}
	return c.Publish(ctx, corecache.InvalidationTopic, payload).Err()
}
