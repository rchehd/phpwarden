package progress

import "context"

type reporterKey struct{}

func WithReporter(ctx context.Context, reporter func(string)) context.Context {
	return context.WithValue(ctx, reporterKey{}, reporter)
}

func Stage(ctx context.Context, label string) {
	if reporter, ok := ctx.Value(reporterKey{}).(func(string)); ok {
		reporter(label)
	}
}
