set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

# Run tests in the Docker test stage.
test:
    docker build --target test .

# Run the complete local CI quality gate.
ci: test
    docker build .

# Preview the files included in the Docker build context.
show_dockerignore:
    #!/bin/sh
    set -eu
    LC_ALL=C
    export LC_ALL

    test_dir="$(mktemp -d)"
    trap 'rm -rf "$test_dir"' 0 1 2 3 15
    mkdir -p "$test_dir/context"

    docker build --file - --progress=quiet --output "type=local,dest=$test_dir/context" . >/dev/null <<-'EOF'
    # syntax=docker/dockerfile:1
    #check=skip=CopyIgnoredFile
    FROM scratch
    COPY . /context
    EOF

    find "$test_dir/context/context" \( -type f -o -type l \) -print |
        sed "s#^$test_dir/context/context/##" |
        sort > "$test_dir/included"

    cat "$test_dir/included"
    printf '\n%s\n' '---'
    printf 'Total files:\t%s\n' "$(wc -l < "$test_dir/included" | awk '{print $1}')"
    printf 'Total size:\t%s\n' "$(du -sh "$test_dir/context/context" | awk '{print $1}')"
