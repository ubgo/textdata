# Typed configuration

How to turn the string values of a `.env` into a typed Go struct (`int`, `bool`, `time.Duration`, slices), with the standard library or with a struct-tag binder. You want this as soon as an application reads more than a couple of settings and a bad value must stop it at startup instead of running with a wrong default.

A `.env` has no types: `PORT=8443` is four characters. Which Go type it becomes depends on the field that receives it, which is your application's decision, so the library hands you strings and you bind them, the same way `encoding/json` unmarshals into a struct instead of offering `json.GetInt`. There is deliberately no `f.Int("PORT", 8080)`: a typed getter that returns its default for `PORT=eighty` brings the service up on the wrong port with nothing in the logs. A binder reports the bad value as an error, together with every other problem in the file. Guessing types from a value's shape was rejected too: `ZIP=01234` would lose its leading zero, and `DESCRIPTION=Hello, world` would become a list.

## Before you start

```sh
go get github.com/ubgo/textdata/dotenv@latest
```

Every snippet is from the runnable example [`examples/dotenv/typed-config`](../../../examples/dotenv/typed-config/), which uses only the standard library. If you choose a binder, also `go get` it (`github.com/sethvargo/go-envconfig` or `github.com/caarlos0/env/v11`); dotenv depends on none of them, and none of them depends on dotenv. The example binds this struct:

<!-- ds:block id=configtype-j7tq2m9w -->
**Config** · [`examples/dotenv/typed-config/main.go:49-55`](../../../examples/dotenv/typed-config/main.go#L49-L55)

```go
type Config struct {
	Port  int
	Debug bool
	Hosts []string
	Wait  time.Duration
	Token string
}
```
<!-- /ds:block hash=b1a45e -->

## Step 1 — Get the expanded values

Bind the **expanded** values, so `${DOMAIN}` references, defaults and plugins have already been applied. From a path, `dotenv.Read(path)` returns them as a `map[string]string`. From a `*File` you already hold (from `Open` or `Parse`), call `f.ExpandedMap()`. Both return an error for an unsatisfied `${VAR:?message}` or a failing plugin; check it before binding. That is how a file can mark a value required on its own, before any binder runs.

Pass the same options you would anywhere else: `dotenv.Read(".env", dotenv.WithLookup(os.LookupEnv))` lets the file reference process environment variables ([plugins.md](plugins.md#step-1--install-one-function-hooks)).

<!-- ds:block id=load-c5hx8n3v -->
**load** · [`examples/dotenv/typed-config/main.go:96-102`](../../../examples/dotenv/typed-config/main.go#L96-L102)

```go
func load(path string) (Config, error) {
	m, err := dotenv.Read(path)
	if err != nil {
		return Config{}, err
	}
	return bind(m)
}
```
<!-- /ds:block hash=744804 -->

## Step 2 — Bind them with the standard library

You do not need a library for a handful of fields: `strconv` and `time.ParseDuration` do the conversion, and `errors.Join` reports every problem at once. Defaults apply only when a key is absent; a key that is present but malformed is an error, never a silent default. The key names are constants, so a typo is a compile error rather than a setting that is never read.

<!-- ds:block id=bind-r2wk6f4p -->
**bind** · [`examples/dotenv/typed-config/main.go:108-140`](../../../examples/dotenv/typed-config/main.go#L108-L140)

```go
func bind(m map[string]string) (Config, error) {
	cfg := Config{Port: defaultPort, Wait: defaultWait}
	var errs []error
	if v, ok := m[keyPort]; ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %q is not an integer", keyPort, v))
		}
		cfg.Port = n
	}
	if v, ok := m[keyDebug]; ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %q is not a boolean", keyDebug, v))
		}
		cfg.Debug = b
	}
	if v, ok := m[keyHosts]; ok && v != "" {
		cfg.Hosts = strings.Split(v, hostsSep)
	}
	if v, ok := m[keyWait]; ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %q is not a duration", keyWait, v))
		}
		cfg.Wait = d
	}
	cfg.Token = m[keyToken]
	if cfg.Token == "" {
		errs = append(errs, fmt.Errorf("%s is required", keyToken))
	}
	return cfg, errors.Join(errs...)
}
```
<!-- /ds:block hash=a7bf06 -->

The good file is `DOMAIN=acme.io`, `PORT=8443`, `DEBUG=true`, `HOSTS=api.${DOMAIN};www.${DOMAIN}`, `TOKEN=t-123`; the bad one is `PORT=eighty`, `WAIT=soon` with no `TOKEN`. See the output in [Try it](#try-it): all three problems in the bad file are reported together.

## Step 3 — Or hand the map to a struct-tag binder

For struct tags, defaults and required fields, hand the map from Step 1 to a binder. Both of these were checked against the example's two files (go-envconfig v1.4.3, caarlos0/env v11.4.1):

| Binder | Call | Bad file |
|---|---|---|
| [`sethvargo/go-envconfig`](https://pkg.go.dev/github.com/sethvargo/go-envconfig) | `envconfig.ProcessWith(ctx, &envconfig.Config{Target: &cfg, Lookuper: envconfig.MapLookuper(m)})` | stops at the first field: `Port: strconv.ParseInt: parsing "eighty": invalid syntax` |
| [`caarlos0/env`](https://pkg.go.dev/github.com/caarlos0/env/v11) | `env.ParseWithOptions(&cfg, env.Options{Environment: m})` | every error together: `env: parse error on field "Port" …; parse error on field "Wait" …; required environment variable "TOKEN" is not set` |

With the good file both produce `{Port:8443 Debug:true Hosts:[api.acme.io www.acme.io] Wait:30s Token:t-123}`, the same as Step 2. When `Environment` (caarlos0) or `Lookuper` (go-envconfig) is set, the process environment is not consulted. go-envconfig versions before v1.0 took the lookuper as a separate argument, `envconfig.ProcessWith(ctx, &cfg, lookuper)`, which does not compile against v1.

[`github.com/ubgo/cfgkit`](https://github.com/ubgo/cfgkit) is a layered configuration system built on this parser: it reads a `.env` / `.env.local` chain through dotenv, so quoting, multi-line values and `${VAR}` expansion behave exactly as in these guides, and adds per-field source reporting, secret masking, validation without booting the application, and `.env.example` generation. Pick go-envconfig or caarlos0/env for a small binder, cfgkit for layered configuration, or Step 2 for no dependency at all.

## Step 4 — Or resolve keys on demand from a File

If you would rather have the binder pull each key from a `*File`, a small adapter satisfies go-envconfig's `Lookuper` interface (`Lookup(key string) (string, bool)`); pass it as `Lookuper: fileLookuper{f}`. That interface has no error return, so the adapter must drop `GetExpanded`'s error, which is the trap the example shows: a failing `${MISSING:?…}` reads as "not set".

<!-- ds:block id=filelookuper-m9dv3k7s -->
**fileLookuper** · [`examples/dotenv/typed-config/main.go:148`](../../../examples/dotenv/typed-config/main.go#L148-L148)

```go
type fileLookuper struct{ f *dotenv.File }
```
<!-- /ds:block hash=b00933 -->

<!-- ds:block id=filelookuperlookup-q6mb3r8z -->
**fileLookuper.Lookup** · [`examples/dotenv/typed-config/main.go:151-157`](../../../examples/dotenv/typed-config/main.go#L151-L157)

```go
func (l fileLookuper) Lookup(key string) (string, bool) {
	v, ok, err := l.f.GetExpanded(key)
	if err != nil {
		return "", false
	}
	return v, ok
}
```
<!-- /ds:block hash=354231 -->

Call `f.ExpandedMap()` once first if a required reference must fail loudly, or use the map form from Step 3.

## Try it

```sh
cd examples/dotenv/typed-config && go run .
```

```text
== .env
{Port:8443 Debug:true Hosts:[api.acme.io www.acme.io] Wait:30s Token:t-123}
== .env.bad
PORT: "eighty" is not an integer
WAIT: "soon" is not a duration
TOKEN is required
== adapter
HOSTS: api.acme.io;www.acme.io true
AUTH: "" false
ExpandedMap: dotenv: required variable MISSING is not set or is empty: set MISSING
```

The module's own version of Step 2 is the tested `Example_typedConfig` in [`dotenv/example_plugin_test.go`](../../../dotenv/example_plugin_test.go).

## Pitfalls

- **Bind expanded values, not raw ones.** `f.Map()` and `f.Get` return values as written, so `HOSTS=api.${DOMAIN}` would reach the binder with the `${DOMAIN}` still in it. Use `Read`, `ExpandedMap` or `GetExpanded`.
- **The per-key adapter hides errors.** It turns a `${VAR:?message}` failure or a secret-store error into "not set", and a field with `default=` then silently takes its default. Call `f.ExpandedMap()` first, or use the map form.
- **`Read` stops at the first expansion error.** A missing `${TOKEN:?…}` aborts the whole read with a `*dotenv.RequiredError` before any binder runs; you see that one error, not a list.
- **An empty value is present.** `PORT=` is in the map with value `""`. Step 2's binder reports it as not an integer; caarlos0/env v11 applies `envDefault` (8080); go-envconfig v1 sets the field to `0` with no error and without applying `default=`. Check how your binder treats empty values.
- **Disabled and inherited lines are not values.** `# PORT=8080` and a bare `PORT` line are not in the map. A name-only `PORT` says the value comes from the process environment, which the library never reads; apply `os.LookupEnv` yourself (see [entries.md](entries.md)).

## Related

- [expansion.md](expansion.md): `${VAR:?message}` and the other operators that decide what each value expands to.
- [plugins.md](plugins.md): `WithLookup(os.LookupEnv)`, secret stores and decryption that run before binding.
- [reading.md](reading.md): `Read`, `Open`, `Parse` and `ExpandedMap`.
- Module README: [Typed configuration](../../../dotenv/README.md#typed-configuration).
- Example: [`examples/dotenv/typed-config`](../../../examples/dotenv/typed-config/).
- API: [pkg.go.dev/github.com/ubgo/textdata/dotenv#Read](https://pkg.go.dev/github.com/ubgo/textdata/dotenv#Read).
