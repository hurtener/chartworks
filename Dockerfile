# Native Rust/CGo build: use the same Debian ABI for build and runtime.
FROM rust:1.98.1-bookworm AS rust
FROM golang:1.26.4-bookworm AS build
COPY --from=rust /usr/local/cargo /usr/local/cargo
COPY --from=rust /usr/local/rustup /usr/local/rustup
ENV CARGO_HOME=/usr/local/cargo RUSTUP_HOME=/usr/local/rustup
ENV PATH=/usr/local/cargo/bin:$PATH
ENV CHARTWORKS_BRUIN_BUILD_DIR=/opt/chartworks-bruin CGO_ENABLED=1
ENV CGO_LDFLAGS=-L/opt/chartworks-bruin/source/pkg/sqlparser/rustffi/target/release
WORKDIR /src
COPY scripts/build-bruin.sh scripts/build-bruin.sh
RUN bash scripts/build-bruin.sh
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN make build && ./bin/chartworks version && ldd ./bin/chartworks

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata libstdc++6 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 chartworks \
    && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin chartworks
COPY --from=build /src/bin/chartworks /usr/local/bin/chartworks
COPY --from=build /opt/chartworks-bruin/bruin /usr/local/libexec/chartworks-bruin
# Version smoke also resolves the actual runtime dynamic libraries.
RUN /usr/local/bin/chartworks version
USER 10001:10001
WORKDIR /tmp
ENTRYPOINT ["/usr/local/bin/chartworks"]
CMD ["version"]
