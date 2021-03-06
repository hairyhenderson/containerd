/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"text/template"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const vendorConf = "vendor.conf"

type note struct {
	Title       string `toml:"title"`
	Description string `toml:"description"`
}

type change struct {
	Commit      string `toml:"commit"`
	Description string `toml:"description"`
}

type dependency struct {
	Name     string
	Commit   string
	Previous string
}

type download struct {
	Filename string
	Hash     string
}

type release struct {
	ProjectName     string            `toml:"project_name"`
	GithubRepo      string            `toml:"github_repo"`
	Commit          string            `toml:"commit"`
	Previous        string            `toml:"previous"`
	PreRelease      bool              `toml:"pre_release"`
	Preface         string            `toml:"preface"`
	Notes           map[string]note   `toml:"notes"`
	BreakingChanges map[string]change `toml:"breaking"`
	// generated fields
	Changes      []change
	Contributors []string
	Dependencies []dependency
	Version      string
	Downloads    []download
}

func main() {
	app := cli.NewApp()
	app.Name = "release"
	app.Description = `release tooling.

This tool should be ran from the root of the project repository for a new release.
`
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "dry,n",
			Usage: "run the release tooling as a dry run to print the release notes to stdout",
		},
		cli.StringFlag{
			Name:  "template,t",
			Usage: "template filepath to use in place of the default",
			Value: defaultTemplateFile,
		},
	}
	app.Action = func(context *cli.Context) error {
		var (
			path = context.Args().First()
			tag  = parseTag(path)
		)
		r, err := loadRelease(path)
		if err != nil {
			return err
		}
		logrus.Infof("Welcome to the %s release tool...", r.ProjectName)
		previous, err := getPreviousDeps(r.Previous)
		if err != nil {
			return err
		}
		changes, err := changelog(r.Previous, r.Commit)
		if err != nil {
			return err
		}
		logrus.Infof("creating new release %s with %d new changes...", tag, len(changes))
		rd, err := fileFromRev(r.Commit, vendorConf)
		if err != nil {
			return err
		}
		deps, err := parseDependencies(rd)
		if err != nil {
			return err
		}
		updatedDeps := updatedDeps(previous, deps)
		contributors, err := getContributors(r.Previous, r.Commit)
		if err != nil {
			return err
		}

		sort.Slice(updatedDeps, func(i, j int) bool {
			return updatedDeps[i].Name < updatedDeps[j].Name
		})

		// update the release fields with generated data
		r.Contributors = contributors
		r.Dependencies = updatedDeps
		r.Changes = changes
		r.Version = tag

		tmpl, err := getTemplate(context)
		if err != nil {
			return err
		}

		if context.Bool("dry") {
			t, err := template.New("release-notes").Parse(tmpl)
			if err != nil {
				return err
			}

			w := tabwriter.NewWriter(os.Stdout, 8, 8, 2, ' ', 0)
			if err := t.Execute(w, r); err != nil {
				return err
			}
			return w.Flush()
		}
		logrus.Info("release complete!")
		return nil
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
