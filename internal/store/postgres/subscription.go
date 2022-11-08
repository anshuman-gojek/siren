package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/odpf/siren/core/subscription"
	"github.com/odpf/siren/internal/store/model"
	"github.com/odpf/siren/pkg/errors"
)

const subscriptionInsertQuery = `
INSERT INTO subscriptions (namespace_id, urn, receiver, match, created_at, updated_at)
    VALUES ($1, $2, $3, $4, now(), now())
RETURNING *
`

const subscriptionUpdateQuery = `
UPDATE subscriptions SET namespace_id=$2, urn=$3, receiver=$4, match=$5, updated_at=now()
WHERE id = $1
RETURNING *
`

const subscriptionDeleteQuery = `
DELETE from subscriptions where id=$1
`

var subscriptionListQueryBuilder = sq.Select(
	"id",
	"namespace_id",
	"urn",
	"receiver",
	"match",
	"created_at",
	"updated_at",
).From("subscriptions")

// SubscriptionRepository talks to the store to read or insert data
type SubscriptionRepository struct {
	client *Client
}

// NewSubscriptionRepository returns SubscriptionRepository struct
func NewSubscriptionRepository(client *Client) *SubscriptionRepository {
	return &SubscriptionRepository{
		client: client,
	}
}

func (r *SubscriptionRepository) List(ctx context.Context, flt subscription.Filter) ([]subscription.Subscription, error) {
	var queryBuilder = subscriptionListQueryBuilder

	// If filter by Labels and namespace ID exist, filter by namespace should be done in app
	// to make use of search by labels with GIN index
	if len(flt.Labels) != 0 {
		labelsJSON, err := json.Marshal(flt.Labels)
		if err != nil {
			return nil, errors.ErrInvalid.WithCausef("problem marshalling json to string with err: %s", err.Error())
		}
		queryBuilder = queryBuilder.Where(fmt.Sprintf("match <@ '%s'::jsonb", string(json.RawMessage(labelsJSON))))
	} else {
		if flt.NamespaceID != 0 {
			queryBuilder = queryBuilder.Where("namespace_id = ?", flt.NamespaceID)
		}
	}

	query, args, err := queryBuilder.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := r.client.GetDB(ctx).QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subscriptionsDomain []subscription.Subscription
	for rows.Next() {
		var subscriptionModel model.Subscription
		if err := rows.StructScan(&subscriptionModel); err != nil {
			return nil, err
		}

		// If filter by Labels and namespace ID exist, filter by namespace should be done in app
		// to make use of search by labels with GIN index
		if len(flt.Labels) != 0 && flt.NamespaceID != 0 {
			if subscriptionModel.NamespaceID != flt.NamespaceID {
				continue
			}
		}

		subscriptionsDomain = append(subscriptionsDomain, *subscriptionModel.ToDomain())
	}

	return subscriptionsDomain, nil
}

func (r *SubscriptionRepository) Create(ctx context.Context, sub *subscription.Subscription) error {
	if sub == nil {
		return errors.New("subscription domain is nil")
	}

	subscriptionModel := new(model.Subscription)
	subscriptionModel.FromDomain(*sub)

	var newSubscriptionModel model.Subscription
	if err := r.client.db.QueryRowxContext(ctx, subscriptionInsertQuery,
		subscriptionModel.NamespaceID,
		subscriptionModel.URN,
		subscriptionModel.Receiver,
		subscriptionModel.Match,
	).StructScan(&newSubscriptionModel); err != nil {
		err := checkPostgresError(err)
		if errors.Is(err, errDuplicateKey) {
			return subscription.ErrDuplicate
		}
		if errors.Is(err, errForeignKeyViolation) {
			return subscription.ErrRelation
		}
		return err
	}

	*sub = *newSubscriptionModel.ToDomain()

	return nil
}

func (r *SubscriptionRepository) Get(ctx context.Context, id uint64) (*subscription.Subscription, error) {
	query, args, err := subscriptionListQueryBuilder.Where("id = ?", id).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, err
	}

	var subscriptionModel model.Subscription
	if err := r.client.db.QueryRowxContext(ctx, query, args...).StructScan(&subscriptionModel); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, subscription.NotFoundError{ID: id}
		}
		return nil, err
	}

	return subscriptionModel.ToDomain(), nil
}

func (r *SubscriptionRepository) Update(ctx context.Context, sub *subscription.Subscription) error {
	if sub == nil {
		return errors.New("subscription domain is nil")
	}

	subscriptionModel := new(model.Subscription)
	subscriptionModel.FromDomain(*sub)

	var newSubscriptionModel model.Subscription
	if err := r.client.db.QueryRowxContext(ctx, subscriptionUpdateQuery,
		subscriptionModel.ID,
		subscriptionModel.NamespaceID,
		subscriptionModel.URN,
		subscriptionModel.Receiver,
		subscriptionModel.Match,
	).StructScan(&newSubscriptionModel); err != nil {
		err := checkPostgresError(err)
		if errors.Is(err, sql.ErrNoRows) {
			return subscription.NotFoundError{ID: subscriptionModel.ID}
		}
		if errors.Is(err, errDuplicateKey) {
			return subscription.ErrDuplicate
		}
		if errors.Is(err, errForeignKeyViolation) {
			return subscription.ErrRelation
		}
		return err
	}

	*sub = *newSubscriptionModel.ToDomain()

	return nil
}

// TODO this won't be synced to provider
func (r *SubscriptionRepository) Delete(ctx context.Context, id uint64) error {
	rows, err := r.client.db.QueryxContext(ctx, subscriptionDeleteQuery, id)
	if err != nil {
		return err
	}
	rows.Close()
	return nil
}
