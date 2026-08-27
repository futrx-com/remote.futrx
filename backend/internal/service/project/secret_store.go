package project

import (
	"context"
	"log"
)

// secretStore owns a project's environment variables and keeps their three
// homes in sync: the durable host store, the `.env` convenience copy in the
// workspace, and the container's LXD environment. Propagation to the last two
// is best-effort — a failure there is logged, never returned, so a stored
// secret is never lost to a container hiccup.
//
// A nil SecretsRepository means secrets are unavailable; a nil
// ContainerEnvironment skips container propagation.
type secretStore struct {
	repo SecretsRepository
	env  ContainerEnvironment
}

func newSecretStore(repo SecretsRepository, env ContainerEnvironment) *secretStore {
	return &secretStore{repo: repo, env: env}
}

func (s *secretStore) list(ctx context.Context, id ID) ([]Secret, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.List(ctx, id)
}

func (s *secretStore) set(ctx context.Context, m Meta, key, value string) (Secret, error) {
	if s.repo == nil {
		return Secret{}, ErrSecretsUnavailable
	}
	saved, err := s.repo.Set(ctx, m.ID, key, value)
	if err != nil {
		return Secret{}, err
	}
	if syncErr := s.syncEnvFile(ctx, m.ID, m.Cwd); syncErr != nil {
		log.Printf("projects: sync .env for %s after set %s: %v", m.ID, key, syncErr)
	}
	if s.env != nil && m.ContainerName != "" {
		if envErr := s.env.ApplyDiff(
			ctx, m.ContainerName,
			map[string]string{key: value}, nil,
		); envErr != nil {
			log.Printf("projects: push env %s to %s: %v", key, m.ContainerName, envErr)
		}
	}
	return saved, nil
}

func (s *secretStore) delete(ctx context.Context, m Meta, key string) error {
	if s.repo == nil {
		return ErrSecretsUnavailable
	}
	if err := s.repo.Delete(ctx, m.ID, key); err != nil {
		return err
	}
	if syncErr := s.syncEnvFile(ctx, m.ID, m.Cwd); syncErr != nil {
		log.Printf("projects: sync .env for %s after delete %s: %v", m.ID, key, syncErr)
	}
	if s.env != nil && m.ContainerName != "" {
		if envErr := s.env.ApplyDiff(
			ctx, m.ContainerName, nil, []string{key},
		); envErr != nil {
			log.Printf("projects: unset env %s on %s: %v", key, m.ContainerName, envErr)
		}
	}
	return nil
}

func (s *secretStore) deleteAll(ctx context.Context, id ID) error {
	if s.repo == nil {
		return nil
	}
	return s.repo.DeleteAll(ctx, id)
}

// syncContainer pushes every stored secret for the project into the
// container's LXD environment.* config.
func (s *secretStore) syncContainer(ctx context.Context, id ID, containerName string) error {
	if s.env == nil || containerName == "" {
		return nil
	}
	if s.repo == nil {
		return nil
	}
	secs, err := s.repo.List(ctx, id)
	if err != nil {
		return err
	}
	if len(secs) == 0 {
		return nil
	}
	set := make(map[string]string, len(secs))
	for _, sec := range secs {
		set[sec.Key] = sec.Value
	}
	return s.env.ApplyDiff(ctx, containerName, set, nil)
}
