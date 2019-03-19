package pipenv

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/cloudfoundry/libcfbuildpack/helper"

	"github.com/cloudfoundry/libcfbuildpack/build"
	"github.com/cloudfoundry/libcfbuildpack/layers"
	"github.com/cloudfoundry/libcfbuildpack/runner"
	"github.com/pkg/errors"
)

const (
	Layer               = "pipenv"
	PythonLayer         = "python"
	PythonPackagesLayer = "python_packages"
)

type PipfileLock struct {
	Meta struct {
		Requires struct {
			Version string `json:"python_version"`
		} `json:"requires"`
	} `json:"_meta"`
}

type Contributor struct {
	context build.Build
	runner  runner.Runner
}

func NewContributor(context build.Build, runner runner.Runner) (Contributor, bool, error) {
	_, willContribute := context.BuildPlan[Layer]
	if !willContribute {
		return Contributor{}, false, nil
	}

	contributor := Contributor{context: context, runner: runner}

	return contributor, true, nil
}

func GetPythonVersionFromPipfileLock(fullPath string) (string, error) {
	file, err := ioutil.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	pipfileLock := PipfileLock{}
	err = json.Unmarshal(file, &pipfileLock)
	if err != nil {
		return "", err
	}

	return pipfileLock.Meta.Requires.Version, nil

}

func (n Contributor) ContributePipenv() error {
	deps, err := n.context.Buildpack.Dependencies()
	if err != nil {
		return err
	}

	dep, err := deps.Best(Layer, "*", n.context.Stack)
	if err != nil {
		return err
	}

	layer := n.context.Layers.DependencyLayer(dep)

	return layer.Contribute(func(artifact string, layer layers.DependencyLayer) error {
		layer.Logger.SubsequentLine("Expanding to %s", layer.Root)
		if err := helper.ExtractTarGz(artifact, layer.Root, 0); err != nil {
			return errors.Wrap(err, "problem extracting")
		}

		if err := n.runner.Run("python", layer.Root, "-m", "pip", "install", "pipenv", "--find-links="+layer.Root); err != nil {
			return errors.Wrap(err, "problem installing pipenv")
		}

		return nil
	}, layers.Build, layers.Cache)
}

func (n Contributor) ContributeRequirementsTxt() error {
	n.context.Logger.Info("Generating requirements.txt")

	if err := n.runner.Run("pipenv", n.context.Application.Root, "lock", "--requirements"); err != nil {
		return errors.Wrap(err, "problem generating initial Pipfile.lock")
	}

	// When we run this a second time, we get the output we care about without extraneous logging
	requirements, err := n.runner.RunWithOutput("pipenv", n.context.Application.Root, "lock", "--requirements")
	if err != nil {
		return errors.Wrap(err, "problem with reading requirements from Pipfile.lock")
	}

	if err = ioutil.WriteFile(filepath.Join(n.context.Application.Root, "requirements.txt"), requirements, 0644); err != nil {
		return errors.Wrap(err, "problem writing requirements")
	}

	return nil
}