package workspaces

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceOwnership(t *testing.T) {
	service := NewService(NewMemoryStore())
	owner := Owner{
		Issuer:  "https://issuer.example",
		Subject: "alice",
	}
	first, err := service.Create(t.Context(), owner, "production")
	require.NoError(t, err)
	second, err := service.Create(t.Context(), owner, "development")
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID)
	assert.Equal(t, owner, first.Owner)
	assert.Equal(t, first.CreatedAt, first.UpdatedAt)
	assert.Equal(t, time.UTC, first.CreatedAt.Location())
	list, err := service.List(t.Context(), owner)
	require.NoError(t, err)
	assert.Equal(t, []Workspace{second, first}, list)
	got, err := service.Get(t.Context(), owner, first.ID.String())
	require.NoError(t, err)
	assert.Equal(t, first, got)

	for _, other := range []Owner{
		{
			Issuer:  owner.Issuer,
			Subject: "bob",
		},
		{
			Issuer:  "https://different-issuer.example",
			Subject: owner.Subject,
		},
	} {
		list, err := service.List(t.Context(), other)
		require.NoError(t, err)
		assert.Empty(t, list)
		got, denied := service.Get(t.Context(), other, first.ID.String())
		require.ErrorIs(t, denied, ErrForbidden)
		assert.Equal(t, Workspace{}, got)
		_, missing := service.Get(t.Context(), other, uuid.NewString())
		require.EqualError(t, missing, denied.Error())
		_, err = service.Create(t.Context(), other, first.Name)
		require.NoError(t, err, "names are unique per owner, not globally")
	}
	_, err = service.Create(t.Context(), owner, first.Name)
	require.ErrorIs(t, err, ErrAlreadyExists)
	require.ErrorIs(t, service.Authorize(t.Context(), owner, "not-a-workspace"), ErrForbidden)

	got.Name = "local-mutation"
	again, err := service.Get(t.Context(), owner, first.ID.String())
	require.NoError(t, err)
	assert.Equal(t, first, again)
}

func TestWorkspaceValidation(t *testing.T) {
	service := NewService(NewMemoryStore())
	owner := Owner{
		Issuer:  "https://issuer.example",
		Subject: "alice",
	}
	for _, invalid := range []Owner{{}, {Issuer: owner.Issuer}, {Subject: owner.Subject}} {
		_, err := service.Create(t.Context(), invalid, "production")
		require.ErrorIs(t, err, ErrUnauthenticated)
		_, err = service.List(t.Context(), invalid)
		require.ErrorIs(t, err, ErrUnauthenticated)
		require.ErrorIs(t, service.Authorize(t.Context(), invalid, uuid.NewString()), ErrUnauthenticated)
	}
	for _, name := range []string{"", " ", " production", "production\nname"} {
		_, err := service.Create(t.Context(), owner, name)
		require.ErrorIs(t, err, ErrInvalid)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := service.Create(ctx, owner, "production")
	require.ErrorIs(t, err, context.Canceled)
	_, err = service.List(ctx, owner)
	require.ErrorIs(t, err, context.Canceled)
	_, err = service.Get(ctx, owner, uuid.NewString())
	require.ErrorIs(t, err, context.Canceled)
}

func TestMemoryStoreValidationAndConcurrency(t *testing.T) {
	store := NewMemoryStore()
	owner := Owner{
		Issuer:  "https://issuer.example",
		Subject: "alice",
	}
	now := time.Now().UTC()
	base := Workspace{
		ID:        uuid.New(),
		Name:      "production",
		Owner:     owner,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, mutate := range []func(*Workspace){
		func(w *Workspace) { w.ID = uuid.Nil },
		func(w *Workspace) { w.CreatedAt = time.Time{} },
		func(w *Workspace) { w.UpdatedAt = now.Add(-time.Second) },
	} {
		invalid := base
		mutate(&invalid)
		require.ErrorIs(t, store.Create(t.Context(), invalid), ErrInvalid)
	}
	invalidOwner := base
	invalidOwner.Owner = Owner{}
	require.ErrorIs(t, store.Create(t.Context(), invalidOwner), ErrUnauthenticated)
	_, err := store.Get(t.Context(), Owner{}, base.ID)
	require.ErrorIs(t, err, ErrUnauthenticated)
	_, err = store.List(t.Context(), Owner{})
	require.ErrorIs(t, err, ErrUnauthenticated)

	results := make(chan error, 20)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			candidate := base
			candidate.ID = uuid.New()
			results <- store.Create(t.Context(), candidate)
		})
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrAlreadyExists)
		}
	}
	assert.Equal(t, 1, successes)
	list, err := store.List(t.Context(), owner)
	require.NoError(t, err)
	require.Len(t, list, 1)
	duplicate := list[0]
	duplicate.Owner.Subject = "bob"
	require.ErrorIs(t, store.Create(t.Context(), duplicate), ErrAlreadyExists)
}
