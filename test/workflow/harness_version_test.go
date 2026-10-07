package workflow

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/praline-labs/rewake/test/workflow/record"
	"github.com/praline-labs/rewake/tools/harnesscache/cache"
	"github.com/praline-labs/rewake/tools/harnesscache/container"
)

// codexVersionEnv names the Codex version the schema is generated from. It is
// read only when the suite is switched on: the five ordinary checks start no
// harness and no container, whatever the environment says.
//
// Unset, the schema case uses the codex on PATH. Set, the version is fetched
// into the harness cache — or found there — before any case starts, and the
// schema is generated from it inside a disposable container, so a version the
// owner has not installed is checked without going near their installation.
//
// A named version that cannot be resolved, fetched or run makes the run red
// with the reason. It is never unsupported and never quietly replaced by the
// installed codex: a run asked about 0.156.0 and answering about 0.155.1 would
// be green about the wrong thing.
const codexVersionEnv = "REWAKE_CODEX_VERSION"

// schemaSource is where the schema comes from in this run. Only the schema
// case and the direct shape tests use a real Codex; every scenario and every
// control runs against the fixture, and the run record says both.
type schemaSource struct {
	// schema is what the run record says about the schema.
	schema record.Against
	// notes are what the fetch said: whether it downloaded or found the
	// version cached.
	notes string
	// absent is set when there is no Codex and none was named, which is
	// unsupported; err when a named version failed, which is red.
	absent string
	err    error
	// generate writes the schema bundles into dir.
	generate func(c *Case, dir string) error
}

// prepareSchemaSource fetches a named version once, in TestMain, before any
// case starts. A first download of a new version takes a minute or more;
// inside a case it ran into that case's deadline, and a download outliving go
// test's own timeout killed the binary with no case to say why. Here it has a
// budget of its own, taken from that timeout.
//
// With no version named nothing happens here: the installed codex is looked
// up only when a case asks for the schema, so a run that selects no schema
// case — the self-check child among them — starts no harness at all.
func prepareSchemaSource() (source schemaSource, prepared bool) {
	selector := os.Getenv(codexVersionEnv)
	if selector == "" {
		return schemaSource{}, false
	}
	ctx, stop := context.WithTimeout(context.Background(), fetchBudget())
	defer stop()
	return namedSchemaSource(ctx, selector), true
}

// fetchBudget is half of go test's -timeout, at most ten minutes: the other
// half is the suite, which takes about three minutes. With no timeout at all
// the ten minutes stand.
func fetchBudget() time.Duration {
	const ceiling = 10 * time.Minute
	timeout := time.Duration(0)
	if f := flag.Lookup("test.timeout"); f != nil {
		if getter, ok := f.Value.(flag.Getter); ok {
			timeout, _ = getter.Get().(time.Duration)
		}
	}
	if timeout <= 0 || timeout/2 > ceiling {
		return ceiling
	}
	return timeout / 2
}

func namedSchemaSource(ctx context.Context, selector string) schemaSource {
	codex, _ := cache.Lookup("codex")
	schema := record.Against{What: "schema", Harness: "codex", Version: selector, How: "named by " + codexVersionEnv + ", in a container"}
	fail := func(err error) schemaSource {
		schema.How += "; could not be used"
		return schemaSource{schema: schema, err: fmt.Errorf("%s=%s: %w", codexVersionEnv, selector, err)}
	}
	root, err := cache.DefaultRoot()
	if err != nil {
		return fail(err)
	}
	if err := container.Mountable(root); err != nil {
		return fail(err)
	}
	var log strings.Builder
	deadline, _ := ctx.Deadline()
	store := &cache.Store{Root: root, Registry: cache.DefaultRegistry, Log: &log, DownloadTimeout: time.Until(deadline)}
	version, err := store.Get(ctx, codex, selector)
	if err != nil {
		return fail(err)
	}
	schema.Version = version.Version
	// Whether this run downloaded, in the line every summary prints: the
	// cache is the owner's explicit request, and the proof that it worked
	// belongs where the result is read.
	if version.Downloaded {
		schema.How += fmt.Sprintf(", downloaded this run (%.0f MB)", float64(version.DownloadedBytes)/1e6)
	} else {
		schema.How += ", from the cache without a download"
	}
	if _, err := container.Ensure(ctx); err != nil {
		return fail(err)
	}
	return schemaSource{
		schema: schema,
		notes:  strings.TrimSpace(log.String()),
		generate: func(c *Case, dir string) error {
			out, err := container.Run(c.Context(), container.Spec{
				Version: version.Dir, Executable: version.ExecutableRel,
				Args:     []string{"app-server", "generate-json-schema", "--experimental", "--out", "/out"},
				Writable: dir, WritableMount: "/out",
			})
			if err != nil {
				return fmt.Errorf("codex %s in the container could not produce a schema: %w: %s", version.Version, err, out)
			}
			return nil
		},
	}
}

// installedSchemaSource is what the case did before versions could be named:
// the codex on PATH, run with the ambient environment. It runs only under the
// suite switch. Its version is asked for only to be named in the record; a
// codex that will not say still produces a schema.
func installedSchemaSource(ctx context.Context) schemaSource {
	binary, err := exec.LookPath("codex")
	if err != nil {
		return schemaSource{
			schema: record.Against{What: "schema", How: "not generated: no codex on PATH"},
			absent: "no codex on PATH, so no schema to check against",
		}
	}
	codex, _ := cache.Lookup("codex")
	version, err := (&cache.Store{}).Resolve(ctx, codex, cache.Installed)
	if err != nil {
		version = "of unknown version (" + err.Error() + ")"
	}
	return schemaSource{
		schema: record.Against{What: "schema", Harness: "codex", Version: version, How: "installed"},
		notes:  "generated by the installed codex " + version,
		generate: func(c *Case, dir string) error {
			// --experimental on purpose: fields behind that flag are exactly
			// the ones the adapter uses — canAcceptDirectInput and
			// runtimeWorkspaceRoots are both experimental — and a schema
			// generated without it describes a narrower protocol than the one
			// being spoken.
			generate := exec.Command(binary, "app-server", "generate-json-schema", "--experimental", "--out", dir)
			generate.Env = os.Environ()
			if out, err := c.Output(generate); err != nil {
				return fmt.Errorf("codex could not produce a schema: %v: %s", err, out)
			}
			return nil
		},
	}
}

// codexSchemaSource answers the source TestMain prepared, and notes that a
// schema was asked for, so the record can tell a generated schema from a
// prepared one no case used.
func codexSchemaSource() schemaSource {
	suite.mu.Lock()
	defer suite.mu.Unlock()
	if !suite.schemaPrepared {
		ctx, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		suite.schema, suite.schemaPrepared = installedSchemaSource(ctx), true
	}
	suite.schemaUsed = true
	return suite.schema
}

// againstForRun is what the run record says the run was checked against, in
// words that match what happened: the schema came from a real Codex, or from
// none; the scenarios, in every column, ran against the fixture.
func againstForRun() []record.Against {
	suite.mu.Lock()
	defer suite.mu.Unlock()
	if !suite.enabled {
		return nil
	}
	schema := suite.schema.schema
	if !suite.schemaUsed && suite.schema.err == nil {
		schema = record.Against{What: "schema", How: "not generated: the schema case did not run"}
	}
	return []record.Against{schema, {What: "scenarios", How: "against the fixture in every column"}}
}
