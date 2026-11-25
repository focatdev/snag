//go:build !windows

package voice

import (
	"context"
	"errors"
)

type Spotter struct{}

func NewSpotter(dir, phrase string, sensitivity float64) (*Spotter, error) {
	return nil, errors.ErrUnsupported
}

func (s *Spotter) Feed(samples []float32) string { return "" }
func (s *Spotter) Reset()                        {}
func (s *Spotter) Close()                        {}

func Listen(ctx context.Context, o Options, onHeard func(phrase string)) error {
	return errors.ErrUnsupported
}
