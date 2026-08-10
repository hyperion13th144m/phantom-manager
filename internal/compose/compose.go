// Package compose drives docker compose for the phantom-release project.
//
// Ported from the old DockerComposeClient.cs, with three changes forced by the
// new phantom-release:
//
//   - Every command carries --env-file .env.docker. Compose no longer picks the
//     settings up implicitly, and without them PHANTOM_SRC_DIR and
//     PHANTOM_DATA_DIR have no values at all.
//   - build and pull are split by what each service actually needs. es is built
//     from infra/es; the other twelve are digest-pinned images.
//   - docker.exe under /mnt/c is gone. Running inside WSL, the docker on PATH
//     is the right one.
package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hyperion13th144m/phantom-manager/internal/envfile"
	"github.com/hyperion13th144m/phantom-manager/internal/runner"
)

// Client runs compose commands against a phantom-release checkout.
type Client struct {
	dir string
}

// New returns a Client for the checkout at releaseDir.
func New(releaseDir string) *Client { return &Client{dir: releaseDir} }

// Service is one row of the service table.
type Service struct {
	Name      string `json:"name"`
	Container string `json:"container,omitempty"`
	Image     string `json:"image,omitempty"`
	State     string `json:"state"`
	Status    string `json:"status"`
	Health    string `json:"health,omitempty"`
	Ports     string `json:"ports"`
	Running   bool   `json:"running"`
}

// Definition describes a service as the compose file declares it.
type Definition struct {
	Name      string `json:"name"`
	Buildable bool   `json:"buildable"` // has a build: section
}

// Ps lists the project's containers, including stopped ones.
func (c *Client) Ps(ctx context.Context) ([]Service, error) {
	if err := c.preflight(); err != nil {
		return nil, err
	}
	out, code := runner.Capture(ctx, c.dir, "docker", c.args("ps", "--all", "--format", "json"))
	if code != 0 {
		return nil, fmt.Errorf("compose ps に失敗しました: %s", firstLine(out))
	}
	return parsePs(out)
}

// Definitions lists the declared services and which of them are built locally.
//
// The split is read from the compose file rather than hardcoded to "es": if
// phantom-release ever builds something else, build and pull follow along.
func (c *Client) Definitions(ctx context.Context) ([]Definition, error) {
	if err := c.preflight(); err != nil {
		return nil, err
	}
	out, code := runner.Capture(ctx, c.dir, "docker", c.args("config", "--format", "json"))
	if code != 0 {
		return nil, fmt.Errorf("compose config に失敗しました: %s", firstLine(out))
	}
	var cfg struct {
		Services map[string]struct {
			Build json.RawMessage `json:"build"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		return nil, fmt.Errorf("compose config の出力を解釈できませんでした: %w", err)
	}
	defs := make([]Definition, 0, len(cfg.Services))
	for name, svc := range cfg.Services {
		defs = append(defs, Definition{Name: name, Buildable: len(svc.Build) > 0})
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs, nil
}

// Build builds the services that have a build section.
func (c *Client) Build(ctx context.Context, log func(runner.Line)) error {
	names, err := c.split(ctx, true)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("ビルド対象のサービスがありません")
	}
	return c.run(ctx, log, append([]string{"build"}, names...)...)
}

// Pull fetches the services that come from a registry.
//
// Naming them explicitly is not an optimisation. A plain `compose pull` also
// tries to pull es, whose image is built locally and exists in no registry, and
// the whole command fails with "pull access denied" — confirmed on both 2.16
// and 5.3.1. Compose grew --ignore-buildable for this in 2.22, but listing the
// services works the same on every version and needs no feature detection.
func (c *Client) Pull(ctx context.Context, log func(runner.Line)) error {
	names, err := c.split(ctx, false)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("取得対象のサービスがありません")
	}
	return c.run(ctx, log, append([]string{"pull"}, names...)...)
}

// Up starts the project in the background.
func (c *Client) Up(ctx context.Context, log func(runner.Line)) error {
	// The bind mount targets have to exist first. Docker creates a missing one
	// as root, and the containers run as uid 1000, so the mistake surfaces much
	// later as a permission error inside a service rather than here.
	settings, _, err := envfile.Load(c.dir)
	if err != nil {
		return err
	}
	if err := settings.EnsureDirs(); err != nil {
		return err
	}
	return c.run(ctx, log, "up", "-d")
}

// Down stops the project and removes its containers.
func (c *Client) Down(ctx context.Context, log func(runner.Line)) error {
	return c.run(ctx, log, "down")
}

// split returns the service names on one side of the build/pull divide.
func (c *Client) split(ctx context.Context, buildable bool) ([]string, error) {
	defs, err := c.Definitions(ctx)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, d := range defs {
		if d.Buildable == buildable {
			names = append(names, d.Name)
		}
	}
	return names, nil
}

// args prefixes a subcommand with the compose invocation and the env file.
func (c *Client) args(sub ...string) []string {
	return append([]string{"compose", "--env-file", envfile.Name}, sub...)
}

func (c *Client) run(ctx context.Context, log func(runner.Line), sub ...string) error {
	if err := c.preflight(); err != nil {
		return err
	}
	args := c.args(sub...)
	res, err := runner.Run(ctx, c.dir, "docker", args, log)
	if err != nil {
		return err
	}
	if !res.OK() {
		return runner.Errorf("docker", args, res)
	}
	return nil
}

// preflight fails early with something actionable. Compose's own message for a
// missing env file names a path relative to its working directory, which is not
// where the user would look.
func (c *Client) preflight() error {
	if _, err := os.Stat(c.dir); err != nil {
		return fmt.Errorf("phantom-release が見つかりません: %s", c.dir)
	}
	if !envfile.Exists(c.dir) {
		return fmt.Errorf("%s がありません。データディレクトリを設定して保存してください", envfile.Path(c.dir))
	}
	return nil
}

// psEntry mirrors the fields compose reports per container.
type psEntry struct {
	Name       string      `json:"Name"`
	Image      string      `json:"Image"`
	Service    string      `json:"Service"`
	State      string      `json:"State"`
	Status     string      `json:"Status"`
	Health     string      `json:"Health"`
	Ports      string      `json:"Ports"`
	Publishers []publisher `json:"Publishers"`
}

type publisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

// parsePs reads both shapes compose has used for --format json.
//
// Compose 2.21 switched to one JSON object per line. Before that — including
// the 2.16 that Docker Desktop installs here — it printed a single JSON array.
// The old manager only understood the line-per-object form, which would have
// silently produced an empty service table on this machine.
// It always returns a non-nil slice on success. A nil slice marshals to JSON
// null rather than [], and the browser has no containers to distinguish that
// from a failure — it just crashes on the first method call.
func parsePs(out string) ([]Service, error) {
	text := strings.TrimSpace(out)
	if text == "" {
		return []Service{}, nil
	}

	var entries []psEntry
	if strings.HasPrefix(text, "[") {
		if err := json.Unmarshal([]byte(text), &entries); err != nil {
			return nil, fmt.Errorf("compose ps の出力を解釈できませんでした: %w", err)
		}
	} else {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var e psEntry
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				return nil, fmt.Errorf("compose ps の出力を解釈できませんでした: %w", err)
			}
			entries = append(entries, e)
		}
	}

	services := make([]Service, 0, len(entries))
	for _, e := range entries {
		name := e.Service
		if name == "" {
			name = e.Name
		}
		services = append(services, Service{
			Name:      name,
			Container: e.Name,
			Image:     e.Image,
			State:     e.State,
			Status:    e.Status,
			Health:    e.Health,
			Ports:     formatPorts(e),
			Running:   strings.EqualFold(e.State, "running"),
		})
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return services, nil
}

// formatPorts renders the published ports.
//
// Publishers is preferred over the Ports string on every compose version, even
// though newer ones fill Ports in. A published port is listed once per address
// family, and both representations carry the duplicate: compose 5.3.1 reports
// Ports as "0.0.0.0:8080->8080/tcp, [::]:8080->8080/tcp" for a single mapping.
// Building from Publishers lets the pair collapse to one entry, and gives the
// same compact result on 2.16, which has no Ports field at all.
//
// The Ports string is still the fallback for the case where compose reports it
// without any Publishers.
func formatPorts(e psEntry) string {
	if len(e.Publishers) == 0 {
		return strings.TrimSpace(e.Ports)
	}
	var parts []string
	seen := map[string]bool{}
	for _, p := range e.Publishers {
		var s string
		switch {
		case p.PublishedPort > 0:
			s = fmt.Sprintf("%d->%d/%s", p.PublishedPort, p.TargetPort, p.Protocol)
		case p.TargetPort > 0:
			s = fmt.Sprintf("%d/%s", p.TargetPort, p.Protocol)
		default:
			continue
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}
