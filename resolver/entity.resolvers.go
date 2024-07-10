package resolver

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.49

import (
	"context"
	"fmt"

	graphql1 "github.com/nebucloud/nginx-service/graphql"
	"github.com/nebucloud/nginx-service/graphql/model"
)

// FindNginxConfigByID is the resolver for the findNginxConfigByID field.
func (r *entityResolver) FindNginxConfigByID(ctx context.Context, id string) (*model.NginxConfig, error) {
	panic(fmt.Errorf("not implemented: FindNginxConfigByID - findNginxConfigByID"))
}

// Entity returns graphql1.EntityResolver implementation.
func (r *Resolver) Entity() graphql1.EntityResolver { return &entityResolver{r} }

type entityResolver struct{ *Resolver }