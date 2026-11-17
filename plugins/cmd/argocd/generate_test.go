//go:build !short

// The "go generate" directives in this file can be skipped by specifying the
// "short" build tag - for example:
//  $ go generate -tags short ./...

//go:generate go run ./testdata/gen
//go:generate go run ./testdata/gen -in testdata/app-of-apps/input.yaml -out testdata/app-of-apps
//go:generate go run ./testdata/gen -in testdata/cluster-scoped/input.yaml -out testdata/cluster-scoped
//go:generate go run ./testdata/gen -in testdata/oci/input.yaml -out testdata/oci

package main
