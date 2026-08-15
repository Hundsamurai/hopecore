package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestGoDirectiveFitsDockerImage сверяет требование go.mod с версией Go в образе.
//
// Тест появился после реального сбоя: `go get` подтянул новую версию
// golang.org/x/net, а `go mod tidy` молча поднял директиву go с 1.22 до 1.25.
// Локально это не проявилось — Go сам скачал нужный тулчейн, — а сборка образа
// упала на `go mod download`, потому что в alpine скачивание запрещено.
func TestGoDirectiveFitsDockerImage(t *testing.T) {
	goMod := readFile(t, "../../go.mod")
	dockerfile := readFile(t, "../../Dockerfile")

	required := parseGoDirective(t, goMod)
	image := parseImageVersion(t, dockerfile)

	if !fitsIn(required, image) {
		t.Fatalf(
			"go.mod требует Go %s, а образ в Dockerfile — golang:%s.\n"+
				"Либо понизьте зависимость, либо поднимите образ: иначе сборка упадёт "+
				"на `go mod download`, хотя локально тесты пройдут за счёт автоскачивания тулчейна.",
			strings.Join(required, "."), strings.Join(image, "."),
		)
	}
}

// TestNoToolchainDirective следит, чтобы в go.mod не появилась строка toolchain.
// Она заставляет Go скачивать конкретную версию, что в образе невозможно.
func TestNoToolchainDirective(t *testing.T) {
	for _, line := range strings.Split(readFile(t, "../../go.mod"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "toolchain ") {
			t.Fatalf("в go.mod появилась директива %q: удалите её, "+
				"иначе сборка образа потребует скачивания тулчейна", strings.TrimSpace(line))
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("не удалось прочитать %s: %v", path, err)
	}
	return string(raw)
}

var goDirective = regexp.MustCompile(`(?m)^go\s+(\d+)\.(\d+)`)

func parseGoDirective(t *testing.T, goMod string) []string {
	t.Helper()

	match := goDirective.FindStringSubmatch(goMod)
	if match == nil {
		t.Fatal("в go.mod нет директивы go")
	}
	return match[1:3]
}

var imageVersion = regexp.MustCompile(`FROM\s+golang:(\d+)\.(\d+)`)

func parseImageVersion(t *testing.T, dockerfile string) []string {
	t.Helper()

	match := imageVersion.FindStringSubmatch(dockerfile)
	if match == nil {
		t.Fatal("в Dockerfile не найден образ golang:X.Y")
	}
	return match[1:3]
}

// fitsIn сообщает, соберётся ли модуль указанным тулчейном.
func fitsIn(required, image []string) bool {
	requiredMajor, requiredMinor := atoi(required[0]), atoi(required[1])
	imageMajor, imageMinor := atoi(image[0]), atoi(image[1])

	if requiredMajor != imageMajor {
		return requiredMajor < imageMajor
	}
	return requiredMinor <= imageMinor
}

func atoi(value string) int {
	number, _ := strconv.Atoi(value)
	return number
}
