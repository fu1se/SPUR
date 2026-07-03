// Package spurmobile is the gomobile-bind facade exposing spur's client
// core to the Android app (android/app). It is deliberately NOT under
// internal/ — see CLAUDE.md's "Android-клиент" section for why: gomobile
// bind generates Go glue code that imports this package by its real
// import path, but that glue code physically lives outside the directory
// tree rooted at internal/'s parent, so the bound package itself must be
// importable from there, which internal/ packages by definition are not.
//
// Every exported name in this package must stick to types gomobile bind
// can translate to Kotlin: bool, numeric types, string, []byte, error,
// and other gomobile-bound interfaces/structs — no context.Context,
// no netip.*/[N]byte fixed arrays, no slices of structs, no bare func
// types for callbacks (single-method interfaces instead), and no more
// than one non-error return value. Internally it's free to call anything
// under internal/, including internal/adapter/rendezvous.
package spurmobile

// Version reports spurmobile's build version, so the Android app can
// display/log it and compare against a connected server's version the
// same way the desktop CLI does (see rendezvous.WarnIfVersionMismatch).
// Sits behind a package-level var, filled in at build time the same way
// cli.version is (see Makefile's mobile-aar target) — "dev" is the
// unbuilt/local-build fallback.
var version = "dev"

func Version() string { return version }
