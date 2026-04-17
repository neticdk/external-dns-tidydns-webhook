#!/usr/bin/env bash
# This file is updated automatically by hack/do.sh. Do not edit it directly.

set -eu

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

NOLINT=${NOLINT:-0}
DEBUG=${DEBUG:-}
FORCE=${FORCE:-}

export TERM=xterm-256color

reset=$(tput sgr0)
orangebold=$(
    tput bold
    tput setaf 166
)
cyan=$(
    tput bold
    tput setaf 6
)
brightred=$(
    tput bold
    tput setaf 196
)

error() {
    echo -e "${brightred}ERROR:${reset} $*"
}

info() {
    echo -e "${cyan}INFO:${reset} $*"
}

notice() {
    echo -e "${orangebold}NOTICE:${reset} $*"
}

set_version() {
    VERSION=$(git describe --tags --always --match=v* 2>/dev/null || echo v0 | sed -e s/^v//)
}

set_default_go_opts() {
    export CGO_ENABLED=0
    DEFAULT_GO_OPTS="-v -tags release -ldflags '-s -w -X main.version=${VERSION}'"
}

set_github_credentials() {
    GITHUB_USER=$(printf "protocol=https\\nhost=github.com\\n" | git credential-manager get | grep username | cut -d= -f2)
    GITHUB_TOKEN=$(printf "protocol=https\\nhost=github.com\\n" | git credential-manager get | grep password | cut -d= -f2)
}

clean() { # cmd: run go clean
    go clean
}

fmt() { # cmd: run go fmt
    go fmt ./...
}

dev_deps() { # cmd: install development dependencies
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
}

lint() { # cmd: run golangci-lint
    which golangci-lint >/dev/null || dev_deps
    golangci-lint run ./...
}

lint_more() { # cmd: run golangci-lint with more lintern enabled
    which golangci-lint >/dev/null || dev_deps
    golangci-lint run --enable gocognit,cyclop,funlen,gocyclo
}

gen() { # cmd: run go generate
    go generate ./...
}

test() { # cmd: run go test
    go test -cover ./... "$@"
}

race() { # cmd: run go test -race
    go test -cover -race ./... "$@"
}

bench() { # cmd: run go test -bench
    go test -bench=./...
}

vet() { # cmd: go run vet
    go vet ./...
}

act() { # cmd: run act (local GitHub Actions runner)
    command act -P ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest
}

docs() { # cmd: generate documentation
    go run . gendocs docs
}

completions() { # cmd: generate shell completions
    DISCOVER_BINARY_NAME=${DISCOVER_BINARY_NAME:-true}
    if [ "${DISCOVER_BINARY_NAME}" = "true" ]; then
        if [ -f go.mod ]; then
            BINARY_NAME=$(grep -E '^module ' go.mod | awk -F/ '{print $NF}')
        else
            BINARY_NAME=$(basename "$(pwd)")
        fi
    fi
    if [ -z "${BINARY_NAME}" ]; then
        echo "BINARY_NAME is not set. Please set it to the name of the binary."
        exit 1
    fi
    rm -rf completions
    mkdir completions
    for sh in bash zsh fish; do
        go run . completion "$sh" >"completions/${BINARY_NAME}.${sh}"
    done
}

build() { # cmd: run go build
    set_version
    set_default_go_opts
    clean
    fmt
    [ "${NOLINT}" -eq 0 ] && lint
    eval go build -o bin/ "$DEFAULT_GO_OPTS" "$*"
}

docker_build() { # cmd: build Docker image
    set_version
    set_github_credentials
    if [ -z "${IMAGE}" ]; then
        echo "IMAGE is not set. Please set it to the name of the image."
        exit 1
    fi
    docker buildx build \
        --progress plain \
        --build-arg GITHUB_USERNAME="${GITHUB_USER}" \
        --build-arg GITHUB_TOKEN="${GITHUB_TOKEN}" \
        --build-arg VERSION="${VERSION}" \
        -t "${IMAGE}":latest \
        -t "${IMAGE}":"${VERSION}" \
        --load .
}

docker_push() { # cmd: push Docker image
    set_version
    set_github_credentials
    if [ -z "${IMAGE}" ]; then
        echo "IMAGE is not set. Please set it to the name of the image."
        exit 1
    fi
    docker buildx build \
        --progress plain \
        --build-arg GITHUB_USERNAME="${GITHUB_USER}" \
        --build-arg GITHUB_TOKEN="${GITHUB_TOKEN}" \
        --build-arg VERSION="${VERSION}" \
        -t "${IMAGE}":latest \
        -t "${IMAGE}":"${VERSION}" \
        --push .
}

install() { # cmd: run go install
    set_version
    set_default_go_opts
    eval go install "$DEFAULT_GO_OPTS" "$*"
}

next_version() { # cmd: get the next sementic version (patch or minor)
    git fetch --all --tags

    latest_tag=$(git tag --sort='v:refname' | tail -1)
    latest_version=$(echo "${latest_tag}" | tr -d 'v')

    if [ -n "${DEBUG}" ]; then
        printf "%-14s : %10s\n" "Latest tag" "${latest_tag}"
        printf "%-14s : %10s\n" "Latest version" "${latest_version}"
    fi

    OIFS="$IFS"
    IFS="." read -r -a semver <<<"${latest_version}"
    IFS="$OIFS"

    # Assume patch version bump if no argument given
    if [ -z "$1" ] || [ "$1" = "patch" ]; then
        _=$((semver[2]++))
    elif [ "$1" = "minor" ]; then
        _=$((semver[1]++))
        semver[2]=0
    fi

    next_version="${semver[0]}.${semver[1]}.${semver[2]}"

    if [ -n "${DEBUG}" ]; then
        printf "%-14s : %10s\n" "Next version" "${next_version}"
    fi

    if [[ ! ${next_version} =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo >&2 "${next_version} is not valid"
        exit 1
    fi

    echo "${next_version}"
}

release() { # cmd: bump tag to next version and push
    bump="${1:-patch}"
    case "$bump" in
    patch | minor)
        next_v=$(next_version "${bump}")

        echo "Next version is: ${next_v}"

        if [ -z "${FORCE}" ]; then
            (
                cd "${SCRIPT_DIR}/.."
                git update-index -q --ignore-submodules --refresh
                err=0

                echo "Checking for unstaged changes..."
                if ! git diff-files --quiet --ignore-submodules --; then
                    echo >&2 "Cannot tag release: you have unstaged changes."
                    git diff-files --name-status -r --ignore-submodules -- >&2
                    err=1
                fi

                echo "Checking for uncommitted changes..."
                if ! git diff-index --cached --quiet HEAD --ignore-submodules --; then
                    echo >&2 "Cannot tag release: your index contains uncommitted changes."
                    git diff-index --cached --name-status -r --ignore-submodules HEAD -- >&2
                    err=1
                fi

                if [ $err = 1 ]; then
                    echo >&2 "Please commit or stash them."
                    exit 1
                fi
            )
        fi

        echo "Pushing release..."
        (
            cd "${SCRIPT_DIR}/.."
            git push
        )

        echo "Tagging release..."
        (
            cd "${SCRIPT_DIR}/.."
            git tag "v${next_v}"
            git push --tags
        )

        echo "Done"
        ;;
    *)
        echo "unsupported release type: ${bump}"
        exit 1
        ;;
    esac
}

fetch_boilerplate_source() {
    local file="$1"
    local dest_name="$2"
    local sha="$3"
    local dest="${SCRIPT_DIR}/../${dest_name}"

    if [ -z "${GITHUB_TOKEN:-}" ]; then
        error "GITHUB_TOKEN is not set. Please set it to your GitHub token."
        exit 1
    fi

    if [ -f "${dest}" ]; then
        if grep -qE '^#.*do.sh:disable-updates' "${dest}"; then
            notice "File ${dest_name} is ignored. Skipping..."
            return
        fi
    fi

    info "Fetching ${file} -> ${dest_name}"
    curl -sSL -H "Authorization: token $GITHUB_TOKEN" "https://raw.githubusercontent.com/neticdk/go-project-boilerplate-source/${sha}/${file}" -o "${dest}.tmp"
    if diff "${dest}.tmp" "${dest}"; then
        rm -f "${dest}.tmp"
    else
        mv "${dest}.tmp" "${dest}"
        notice "Updated ${dest_name}"
    fi

    # Set executable bit if the file is a script
    if head -1 "${dest}" | grep -qE '^#!'; then
        chmod 755 "${dest}"
    fi
}

renovate() { # cmd: update boilerplate from hack/boilerplate.json
    local ref
    if [ ! -f "${SCRIPT_DIR}/boilerplate.json" ]; then
        error "hack/boilerplate.json not found. Please run update_boilerplate first."
        exit 1
    fi

    ref=$(jq -r .ref "${SCRIPT_DIR}/boilerplate.json")
    if [ -z "${ref}" ]; then
        error "ref not found in hack/boilerplate.json. Please run update_boilerplate first."
        exit 1
    fi

    update_boilerplate_files "${ref}"
}

update_boilerplate_files() {
    local ref="${1:-}"
    local hash_cur
    local hash_new
    local sha

    local md5
    md5=$(which md5sum 2>/dev/null || which md5)

    info "Getting sha for ref: ${ref}..."
    sha=$(curl -s -L \
        -H "Accept: application/vnd.github+json" \
        -H "Authorization: Bearer $GITHUB_TOKEN" \
        -H "X-GitHub-Api-Version: 2022-11-28" \
        "https://api.github.com/repos/neticdk/go-project-boilerplate-source/commits/$ref" | jq -r .sha)

    if [ -z "${sha}" ]; then
        error "Invalid ref: ${ref}"
        exit 1
    fi

    info "Updating boilerplate from ${ref}..."

    hash_cur="$(${md5} "${SCRIPT_DIR}/do.sh" | cut -d' ' -f1)"
    fetch_boilerplate_source "build/hack/do.sh" "hack/do.sh" "${sha}"
    hash_new="$(${md5} "${SCRIPT_DIR}/do.sh" | cut -d' ' -f1)"
    if [ "${hash_cur}" != "${hash_new}" ]; then
        notice "Updated do.sh. Re-running..."
        "${SCRIPT_DIR}/do.sh" update_boilerplate "${ref}"
        exit 0
    fi

    fetch_boilerplate_source "build/golangci.yml" ".golangci.yml" "${sha}"
    fetch_boilerplate_source "build/Makefile" "Makefile" "${sha}"
    fetch_boilerplate_source "develop/editorconfig" ".editorconfig" "${sha}"
    fetch_boilerplate_source "cicd/github/boilerplate.yaml" ".github/workflows/boilerplate.yaml" "${sha}"
}

update_boilerplate() { # cmd: update boilerplate source manually
    local ref="${1:-}"

    if [ -z "$ref" ]; then
        if [ -z "${GITHUB_TOKEN:-}" ]; then
            error "GITHUB_TOKEN is not set. Please set it to your GitHub token."
            exit 1
        fi

        info "Getting latest go-project-boilerplate-source tag..."
        tag=$(curl -s -L \
            -H "Accept: application/vnd.github+json" \
            -H "Authorization: Bearer $GITHUB_TOKEN" \
            -H "X-GitHub-Api-Version: 2022-11-28" \
            "https://api.github.com/repos/neticdk/go-project-boilerplate-source/releases/latest" | jq -r .tag_name)

        if [ -z "${tag}" ] || [ "${tag}" = "null" ]; then
            notice "No tags found. Using main."
            ref="main"
        else
            ref="${tag}"
        fi
    fi

    update_boilerplate_files "${ref}"

    if [ ! -f "${SCRIPT_DIR}/boilerplate.json" ]; then
        notice "hack/boilerplate.json not found. Creating a new one..."
        {
            echo "{"
            echo "  \"datasource\": \"git-refs\","
            echo "  \"depName\": \"github.com/neticdk/go-project-boilerplate-source\","
            echo "  \"ref\": \"${ref}\""
            echo "}"
        } >"${SCRIPT_DIR}/boilerplate.json"
    else
        ref_cur="$(jq -r .ref hack/boilerplate.json)"
        if [ "${ref_cur}" != "${ref}" ]; then
            notice "Updating hack/boilerplate.json ref from ${ref_cur} to ${ref}"
            cat <<<"$(jq ".ref = \"${ref}\"" hack/boilerplate.json)" >"hack/boilerplate.json"
        fi
    fi
}

functions=$(compgen -A function)

usage() {
    echo "Usage: $0 <command> [args]"
    echo "Available commands:"
    grep -E '.*\(\) { # cmd:' "$0" | sort | sed -E "s/^(.*)\(\) { # cmd: (.*)/\1 - \2/"
}

# This allows for for overrides and extending the script with custom commands
if [ -f "${SCRIPT_DIR}/hack/do-custom.sh" ]; then
    source "${SCRIPT_DIR}/hack/do-custom.sh"
fi

if [ $# -eq 0 ] || [ "${1:-}" = "--help" ]; then
    usage
    exit 1
fi

OIFS="$IFS"
IFS=$'\n'
found=0
for f in $functions; do
    if [ "$f" = "$1" ]; then
        found=1
        break
    fi
done
IFS="$OIFS"

if [ $found -eq 0 ]; then
    error "Unknown command: $1"
    usage
    exit 1
fi

"$@"
