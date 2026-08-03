<p align="center">
  <img src="assets/logo.svg" width="128" alt="mvn.sh logo">
</p>

<h1 align="center">mvn.sh CLI</h1>

The official command-line client for [mvn.sh](https://mvn.sh). It installs an authenticated mvn.sh repository profile in Maven's `settings.xml`.

## Install

### Installer (Linux and macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/mvn-sh/cli/main/install.sh | sh
```

The installer verifies the release checksum and places `mvnsh` in `~/.local/bin`. Set `MVNSH_INSTALL_DIR` to choose another directory.

### Windows

Download the MSI package from [GitHub Releases](https://github.com/mvn-sh/cli/releases), or install from PowerShell:

```powershell
irm https://raw.githubusercontent.com/mvn-sh/cli/main/install.ps1 | iex
```

Both Windows installers add `mvnsh` to your `PATH`.

### Go

```bash
go install github.com/mvn-sh/cli/cmd/mvnsh@latest
```

### Release archive

Download the archive for your platform from [GitHub Releases](https://github.com/mvn-sh/cli/releases), then place the `mvnsh` binary on your `PATH`.

## Usage

```bash
mvnsh login
```

The CLI opens mvn.sh in your browser. Sign in, choose a team and repository, and approve the scoped credential. The CLI then updates `~/.m2/settings.xml` with:

- a Maven server containing the token credentials;
- a repository and plugin repository profile;
- an active-profile entry.

Existing Maven configuration is retained. Managed entries are marked with `mvnsh:` comments and replaced on subsequent logins. The original file is copied to `~/.m2/settings.xml.bak`, and the resulting settings file is restricted to the current user.

For automation, provide an existing scoped token:

```bash
MVN_TOKEN='mvn_…' mvnsh login --team acme --repository releases
```

Run `mvnsh login -h` for all options, including custom Maven settings and profile paths.

Update an installed release at any time:

```bash
mvnsh update
```

## Local development

```bash
go test ./...
go build -o bin/mvnsh ./cmd/mvnsh
```

To use a local mvn.sh stack:

```bash
./bin/mvnsh login \
  --app-url 'http://localhost:3000' \
  --api-url 'http://localhost:8080' \
  --base-url 'http://%s.localhost:8080'
```

## License

[Apache License 2.0](LICENSE)
