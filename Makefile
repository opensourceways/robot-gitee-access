bazel ?= bazel-3.4.1
binary ?= robot-gitee-access

define tips
	$(info )
	$(info *************** $(1) ***************)
	$(info )
endef

.PHONY: load-lib

load-lib:
	$(call tips,Load Lib)
	$(bazel) run //:gazelle -- update-repos -from_file=go.mod

.PHONY: gen-bzl

gen-bzl:
	$(call tips,Generate bazel File)
	$(bazel) run //:gazelle

.PHONY: build

build: load-lib gen-bzl
	$(call tips,Start Build)
	$(bazel) build //:$(binary)

.PHONY: clean

clean:
	$(bazel) clean

.PHONY: image

image:
	$(call tips,Build Image)
	$(bazel) run //:image --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64

.PHONY: image-push

image-push:
	$(call tips,Push Image)
	$(bazel) run //:image-push --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64
