package tests

import (
	"os"
	"path/filepath"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/services/caches"
	"github.com/kuetix/engine/modules"
)

func BootstrapTest() {
	env := domain.NewEnvironment("tests", &domain.Options{
		EngineName:    "tests",
		ConfigName:    "tests",
		Verbose:       true,
		Quiet:         false,
		Amount:        1,
		Retry:         1,
		RetryDelay:    0,
		RestartPolicy: "",
		Workflow:      "",
		Version:       "TestVersion",
		BuildTime:     "TestBuildTime",
		LogPath:       "",
		Config:        nil,
		Args:          nil,
		Context:       nil,
		Settings:      nil,
		EmbedFS:       &WorkflowsEmbedFS,
	})

	var err error
	var dir string

	dir, err = os.Getwd()
	if err != nil {
		panic(err)
	}
	parentDir := filepath.Dir(dir)
	err = os.Chdir(parentDir)
	if err != nil {
		panic(err)
	}
	caches.GenerateMetaCache(env)
	err = os.Chdir(dir)
	if err != nil {
		panic(err)
	}

	app := domain.NewApplication(env)
	app.Env.Config.Application.Name = "tests"
	app.Env.Config.Application.WorkflowsPath = "./workflows/"

	modules.Enable()
	di.DependencyInjectionBoot()

	defer app.Close()

	cwdTests := env.GetEnvPath()
	err = os.Chdir(cwdTests)
	if err != nil {
		return
	}
}
