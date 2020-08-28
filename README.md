# GCS Cacher

[![GoDoc](https://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)](https://pkg.go.dev/mod/github.com/sethvargo/gcs-cacher)
[![GitHub Actions](https://img.shields.io/github/workflow/status/sethvargo/gcs-cacher/Test?style=flat-square)](https://github.com/sethvargo/gcs-cacher/actions?query=workflow%3ATest)

GCS Cacher is a small CLI and Docker container that saves and restores caches on
[Google Cloud Storage][gcs]. It is intended to be used in CI/CD systems like
[Cloud Build][gcb], but may have applications elsewhere.


## Usage

1.  Create a cache:

    ```shell
    gcs-cacher -bucket "my-bucket" -cache "go-mod" -dir "$GOPATH/pkg/mod"
    ```

    This will compress and upload the contents at `pkg/mod` to Google Cloud
    Storage at the key "go-mod".

1.  Restore a cache:

    ```shell
    gcs-cacher -bucket "my-bucket" -restore "go-mod" -dir "$GOPATH/pkg/mod"
    ```

    This will download the Google Cloud Storage object named "go-mod" and
    decompress it to `pkg/mod`.


## Installation

Choose from one of the following:

-   Download the latest version from the [releases][releases].

-   Use a pre-built Docker container:

    ```text
    us-docker.pkg.dev/vargolabs/gcs-cacher/gcs-cacher
    docker.pkg.github.com/sethvargo/gcs-cacher/gcs-cacher
    ```


## Implementation

When saving the cache, the provided directory is made into a tarball, then
gzipped, then uploaded to Google Cloud Storage. When restoring the cache, the
reverse happens.

It's strongly recommend that you use a cache key based on your dependency file,
and restore up the chain. For example:

```shell
gcs-cacher \
  -bucket "my-bucket" \
  -cache "ruby-{{ hashGlob "Gemfile.lock" }}"
```

```shell
gcs-cacher \
  -bucket "my-bucket" \
  -restore "ruby-{{ hashGlob "Gemfile.lock" }}"
  -restore "ruby-"
```

This will maximize cache hits.


[gcs]: https://cloud.google.com/storage
[gcb]: https://cloud.google.com/cloud-build
[releases]: releases
