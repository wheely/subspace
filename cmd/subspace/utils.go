package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"text/template"
	"time"
)

func RandomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.URLEncoding.EncodeToString(b)[:n]
}

func Overwrite(filename string, data []byte, perm os.FileMode) error {
	f, err := ioutil.TempFile(filepath.Dir(filename), filepath.Base(filename)+".tmp")
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(f.Name(), perm); err != nil {
		return err
	}
	return os.Rename(f.Name(), filename)
}

func bash(tmpl string, params interface{}) (string, error) {
	preamble := `
set -o nounset
set -o errexit
set -o pipefail
set -o xtrace
`
	t, err := template.New("template").Parse(preamble + tmpl)
	if err != nil {
		return "", err
	}
	var script bytes.Buffer
	err = t.Execute(&script, params)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	output, err := exec.CommandContext(ctx, "/bin/bash", "-c", string(script.Bytes())).CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %s\n%s", err, string(output))
	}
	return string(output), nil
}

func FindFirstFreeID(profiles []*Profile) (freeID uint32) {
	profileIDs := getProfileIDs(profiles)
	logger.Debugf("profileIDS: %v", profileIDs)
	sort.Slice(profileIDs, func(i, j int) bool {
		return profileIDs[i] < profileIDs[j]
	})
	logger.Debugf("Sorted profileIDS: %v", profileIDs)
	const minID = 2
	if len(profileIDs) == 0 {
		return minID
	}
	logger.Debugf("minID: %v", minID)
	maxID := profileIDs[len(profileIDs)-1]
	logger.Debugf("maxID: %v", maxID)
	freeID = uint32(maxID + 1)
	logger.Debugf("freeID: %v", freeID)
	for i := minID; i < maxID; i++ {
		if i != profileIDs[i-minID] {
			freeID = uint32(i)
			break
		}
	}
	logger.Debugf("Return freeID: %v", freeID)
	return
}

func getProfileIDs(profiles []*Profile) []int {
	var profileIDs = make([]int, len(profiles))
	logger.Debugf("Profiles - params: %v", profiles)
	logger.Debugf("make profileIDS: %v", profileIDs)
	for i, profile := range profiles {
		profileIDs[i] = profile.Number
	}
	logger.Debugf("return profileIDS: %v", profileIDs)
	return profileIDs
}
