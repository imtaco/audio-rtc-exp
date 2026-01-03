package workflow

import "context"

func WithEitherDone(a, b context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(a)

	// AfterFunc returns a stop func; call it to release resources.
	stopA := context.AfterFunc(a, cancel)
	stopB := context.AfterFunc(b, cancel)

	return ctx, func() {
		stopA()
		stopB()
		cancel()
	}
}
