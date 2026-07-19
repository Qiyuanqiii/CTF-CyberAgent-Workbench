//go:build !windows

package credential

import "context"

type unsupportedStore struct{}

func newSystemStore() Store                                        { return unsupportedStore{} }
func (unsupportedStore) Kind() string                              { return "unsupported" }
func (unsupportedStore) Available() bool                           { return false }
func (unsupportedStore) Put(context.Context, string, string) error { return ErrUnavailable }
func (unsupportedStore) Delete(context.Context, string) error      { return ErrUnavailable }
func (unsupportedStore) Get(context.Context, string) (string, bool, error) {
	return "", false, ErrUnavailable
}
func (unsupportedStore) Configured(context.Context, string) (bool, error) {
	return false, nil
}
