# docker build . -t cyborgshead/cyber:latest
# docker run --rm -it cyborgshead/cyber:latest /bin/sh

FROM golang:1.23.8-alpine3.21 AS go-builder

# this comes from standard alpine nightly file
#  https://github.com/rust-lang/docker-rust-nightly/blob/master/alpine3.12/Dockerfile
# with some changes to support our toolchain, etc
RUN set -eux; apk add --no-cache ca-certificates build-base;

RUN apk add git
# NOTE: add these to run with LEDGER_ENABLED=true
# RUN apk add libusb-dev linux-headers

WORKDIR /code
COPY . /code/
# See https://github.com/CosmWasm/wasmvm/releases
ADD https://github.com/CosmWasm/wasmvm/releases/download/v2.2.3/libwasmvm_muslc.aarch64.a /lib/libwasmvm_muslc.aarch64.a
ADD https://github.com/CosmWasm/wasmvm/releases/download/v2.2.3/libwasmvm_muslc.x86_64.a /lib/libwasmvm_muslc.x86_64.a
RUN sha256sum /lib/libwasmvm_muslc.aarch64.a | grep 6641730781bb1adc4bdf04a1e0f822b9ad4fb8ed57dcbbf575527e63b791ae41
RUN sha256sum /lib/libwasmvm_muslc.x86_64.a | grep 32503fe35a7be202c5f7c3051497d6e4b3cd83079a61f5a0bf72a2a455b6d820

# force it to use static lib (from above) not standard libgo_cosmwasm.so file
RUN LEDGER_ENABLED=false BUILD_TAGS=muslc LINK_STATICALLY=true make build

# --------------------------------------------------------
FROM alpine:3.21.3

COPY --from=go-builder /code/build/cyber /usr/bin/cyber

COPY docker/* /opt/
RUN chmod +x /opt/*.sh

WORKDIR /opt

EXPOSE 26657 26656 9090 1317 26660

CMD ["/usr/bin/cyber", "version"]
