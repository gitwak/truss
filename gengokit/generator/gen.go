// Package generator generates a gokit service based on a service definition.
package generator

import (
	"bytes"
	"go/format"
	"io"
	"io/ioutil"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"

	"github.com/TuneLab/truss/gengokit"
	"github.com/TuneLab/truss/gengokit/handlers"
	templFiles "github.com/TuneLab/truss/gengokit/template"

	"github.com/TuneLab/truss/gengokit/utils"
	"github.com/TuneLab/truss/kit"
	"github.com/TuneLab/truss/svcdef"
)

// GenerateGokit returns a gokit service generated from a service definition (svcdef),
// the package to the root of the generated service goPackage, the package
// to the .pb.go service struct files (goPBPackage) and any prevously generated files.
func GenerateGokit(sd *svcdef.Svcdef, conf gengokit.Config) (map[string]io.Reader, error) {
	data, err := gengokit.NewData(sd, conf)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create template data")
	}

	codeGenFiles := make(map[string]io.Reader)

	// Remove the suffix "-service" since it's added back in by templatePathToActual
	svcname := strings.ToLower(sd.Service.Name)
	for _, templPath := range templFiles.AssetNames() {
		// Only render the files that match conf.KitVersion
		templKitVersion := utils.GoKitTemplateFPVersion(templPath)
		if kit.Version != templKitVersion {
			continue
		}

		// Re-derive the actual path for this file based on the service output
		// path provided by the truss main.go
		actualPath := utils.TemplatePathToActual(templPath, svcname)
		file, err := generateResponseFile(templPath, actualPath, data, conf.PreviousFiles[actualPath])
		if err != nil {
			return nil, errors.Wrap(err, "cannot render template")
		}
		codeGenFiles[actualPath] = file
	}

	return codeGenFiles, nil
}

// generateResponseFile contains logic to choose how to render a template file
// based on path and if that file was generated previously. It accepts a
// template path to render, a templateExecutor to apply to the template, and a
// map of paths to files for the previous generation. It returns a
// io.Reader representing the generated file.
func generateResponseFile(templFP string, actualFP string, data *gengokit.Data, prevFile io.Reader) (io.Reader, error) {
	var genCode io.Reader
	var err error

	switch actualFP {
	case handlers.ServerHandlerPath:
		h, err := handlers.New(data.Service, prevFile)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot parse previous handler: %q", actualFP)
		}

		if genCode, err = h.Render(templFP, data); err != nil {
			return nil, errors.Wrapf(err, "cannot render template: %s", templFP)
		}
	case handlers.HookPath:
		hook := handlers.NewHook(prevFile)
		if genCode, err = hook.Render(templFP, data); err != nil {
			return nil, errors.Wrapf(err, "cannot render template: %s", templFP)
		}
	case handlers.MiddlewaresPath:
		m := handlers.NewMiddlewares()
		m.Load(prevFile)
		if genCode, err = m.Render(templFP, data); err != nil {
			return nil, errors.Wrapf(err, "cannot render template: %s", templFP)
		}
	default:
		if genCode, err = applyTemplateFromPath(templFP, data); err != nil {
			return nil, errors.Wrapf(err, "cannot render template: %s", templFP)
		}
	}

	codeBytes, err := ioutil.ReadAll(genCode)
	if err != nil {
		return nil, err
	}

	// ignore error as we want to write the code either way to inspect after
	// writing to disk
	formattedCode := formatCode(codeBytes)

	return bytes.NewReader(formattedCode), nil
}

// applyTemplateFromPath calls applyTemplate with the template at templFP
func applyTemplateFromPath(templFP string, data *gengokit.Data) (io.Reader, error) {
	templBytes, err := templFiles.Asset(templFP)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to find template file: %v", templFP)
	}

	return data.ApplyTemplate(string(templBytes), templFP)
}

// formatCode takes a string representing golang code and attempts to return a
// formated copy of that code.  If formatting fails, a warning is logged and
// the original code is returned.
func formatCode(code []byte) []byte {
	formatted, err := format.Source(code)

	if err != nil {
		log.WithError(err).Warn("Code formatting error, generated service will not build, outputting unformatted code")
		// return code so at least we get something to examine
		return code
	}

	return formatted
}
