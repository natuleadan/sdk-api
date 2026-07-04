package openfga

import (
	"context"
	"fmt"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

// Client wraps OpenFGA SDK for authorization checks.
type Client struct {
	fga     *client.OpenFgaClient
	storeID string
}

// Config holds OpenFGA connection settings.
type Config struct {
	APIURL  string
	StoreID string
}

// NewClient creates an OpenFGA client.
func NewClient(cfg Config) (*Client, error) {
	fgaClient, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:  cfg.APIURL,
		StoreId: cfg.StoreID,
	})
	if err != nil {
		return nil, fmt.Errorf("openfga: failed to create client: %w", err)
	}

	return &Client{
		fga:     fgaClient,
		storeID: cfg.StoreID,
	}, nil
}

// CheckRequest defines an authorization check.
type CheckRequest struct {
	User     string
	Relation string
	Object   string
}

// Checker is the interface for authorization checks.
type Checker interface {
	Check(ctx context.Context, req CheckRequest) (bool, error)
}

// Check performs an authorization check against OpenFGA.
func (c *Client) Check(ctx context.Context, req CheckRequest) (bool, error) {
	body := client.ClientCheckRequest{
		User:     req.User,
		Relation: req.Relation,
		Object:   req.Object,
	}

	data, err := c.fga.Check(ctx).Body(body).Execute()
	if err != nil {
		return false, fmt.Errorf("openfga: check failed: %w", err)
	}

	return data.GetAllowed(), nil
}

// WriteTuple writes a relationship tuple to OpenFGA.
func (c *Client) WriteTuple(ctx context.Context, user, relation, object string) error {
	body := client.ClientWriteRequest{
		Writes: []client.ClientTupleKey{
			{
				User:     user,
				Relation: relation,
				Object:   object,
			},
		},
	}

	_, err := c.fga.Write(ctx).Body(body).Execute()
	if err != nil {
		return fmt.Errorf("openfga: write tuple failed: %w", err)
	}

	return nil
}

// DeleteTuple deletes a relationship tuple from OpenFGA.
func (c *Client) DeleteTuple(ctx context.Context, user, relation, object string) error {
	body := client.ClientWriteRequest{
		Deletes: []client.ClientTupleKeyWithoutCondition{
			{
				User:     user,
				Relation: relation,
				Object:   object,
			},
		},
	}

	_, err := c.fga.Write(ctx).Body(body).Execute()
	if err != nil {
		return fmt.Errorf("openfga: delete tuple failed: %w", err)
	}

	return nil
}

// WriteAuthorizationModel writes an authorization model to OpenFGA.
func (c *Client) WriteAuthorizationModel(ctx context.Context, model openfga.WriteAuthorizationModelRequest) (string, error) {
	resp, err := c.fga.WriteAuthorizationModel(ctx).Body(model).Execute()
	if err != nil {
		return "", fmt.Errorf("openfga: write model failed: %w", err)
	}

	return resp.GetAuthorizationModelId(), nil
}
