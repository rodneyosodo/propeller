package plugin

import "context"

type authKey struct{}

// ContextWithAuth stores an AuthContext in ctx.
func ContextWithAuth(ctx context.Context, auth AuthContext) context.Context {
	return context.WithValue(ctx, authKey{}, auth)
}

// AuthFromContext retrieves the AuthContext stored by ContextWithAuth.
// Returns a zero-value AuthContext when none is present.
func AuthFromContext(ctx context.Context) AuthContext {
	v, _ := ctx.Value(authKey{}).(AuthContext)

	return v
}
