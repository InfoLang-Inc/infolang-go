package infolang

// Version is the semantic version of the infolang-go module. It is stamped into
// the default User-Agent header and reported by the CLI. It is declared as a var
// (not a const) so release tooling can override it at build time with
// -ldflags "-X github.com/InfoLang-Inc/infolang-go.Version=<tag>".
var Version = "0.1.0"
