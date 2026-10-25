# Changelog

## [0.29.0](https://github.com/stuttgart-things/clusterscope/compare/v0.28.0...v0.29.0) (2026-10-25)


### Features

* serve under BASE_PATH, /health and embedded header for sthings-portal ([#103](https://github.com/stuttgart-things/clusterscope/issues/103)) ([c0148bb](https://github.com/stuttgart-things/clusterscope/commit/c0148bb5510e119088996e3bf5793c920cb3041d))


### Bug Fixes

* **deps:** port PDF export to the chromedp v0.21 API ([cd2380f](https://github.com/stuttgart-things/clusterscope/commit/cd2380f6dd9789d3e03db02ca45e35179cf8cc7d))
* **deps:** update kubernetes monorepo to v0.37.1 ([2c8c3cc](https://github.com/stuttgart-things/clusterscope/commit/2c8c3ccf04fddf3355e35098cee7022da03579b6))
* **deps:** update module dagger.io/dagger to v0.21.11 ([981dd6f](https://github.com/stuttgart-things/clusterscope/commit/981dd6fda9f082567867a53d1957738ebc877476))
* **deps:** update module github.com/chromedp/cdproto to v0.158.4 ([8fd2b79](https://github.com/stuttgart-things/clusterscope/commit/8fd2b79d1f6bec250fe74d64efd6e8d86c1c75f8))
* **deps:** update module github.com/chromedp/chromedp to v0.21.0 ([718c680](https://github.com/stuttgart-things/clusterscope/commit/718c68044c64229b6cfcc1c2d65c409481fc48f0))
* **deps:** update module github.com/prometheus/client_golang to v1.25.0 ([cd5315d](https://github.com/stuttgart-things/clusterscope/commit/cd5315d84ae108abb6d7bfdeb09fc4f4c779a9a3))
* **deps:** update module github.com/vektah/gqlparser/v2 to v2.5.64 ([76d4c67](https://github.com/stuttgart-things/clusterscope/commit/76d4c67ff969c16408460804f6df19416116c457))

## [0.28.0](https://github.com/stuttgart-things/clusterscope/compare/v0.27.0...v0.28.0) (2026-10-19)


### Features

* **lab:** name the cluster whose Kustomization includes a directory ([#93](https://github.com/stuttgart-things/clusterscope/issues/93)) ([#94](https://github.com/stuttgart-things/clusterscope/issues/94)) ([c0b06fa](https://github.com/stuttgart-things/clusterscope/commit/c0b06fa96ab3dbef746b5a0c0b6783450ecf8bdd))


### Bug Fixes

* **deps:** update module github.com/fsnotify/fsnotify to v1.10.1 ([#80](https://github.com/stuttgart-things/clusterscope/issues/80)) ([5cc770d](https://github.com/stuttgart-things/clusterscope/commit/5cc770d6c5786c17e14a48a4f701889f34807bea))

## [0.27.0](https://github.com/stuttgart-things/clusterscope/compare/v0.26.0...v0.27.0) (2026-10-18)


### Features

* **lab:** machinery as a cluster source -- ClusterStack orders ([#60](https://github.com/stuttgart-things/clusterscope/issues/60)) ([#88](https://github.com/stuttgart-things/clusterscope/issues/88)) ([3bc0194](https://github.com/stuttgart-things/clusterscope/commit/3bc01949812bb13c7bfb57f1bce33cf0b8e737cb))

## [0.26.0](https://github.com/stuttgart-things/clusterscope/compare/v0.25.1...v0.26.0) (2026-10-15)


### Features

* **endpoints:** opt routes out of probing with an annotation ([#85](https://github.com/stuttgart-things/clusterscope/issues/85)) ([878e358](https://github.com/stuttgart-things/clusterscope/commit/878e35829848e97743b0ce37a3d9ba23ef705df8))

## [0.25.1](https://github.com/stuttgart-things/clusterscope/compare/v0.25.0...v0.25.1) (2026-10-15)


### Bug Fixes

* **lab:** do not list a cluster with a directory as kubeconfig only ([#81](https://github.com/stuttgart-things/clusterscope/issues/81)) ([4eae72a](https://github.com/stuttgart-things/clusterscope/commit/4eae72a4b604803705784ea4917b2f0701d9d3a5))

## [0.25.0](https://github.com/stuttgart-things/clusterscope/compare/v0.24.0...v0.25.0) (2026-10-15)


### Features

* **lab:** clusterbook as cluster source ([#78](https://github.com/stuttgart-things/clusterscope/issues/78)) ([528f02e](https://github.com/stuttgart-things/clusterscope/commit/528f02ec5141a8240cde74e3e41eee0267783c30))


### Bug Fixes

* **deps:** update module github.com/vektah/gqlparser/v2 to v2.5.62 ([#65](https://github.com/stuttgart-things/clusterscope/issues/65)) ([595c6c6](https://github.com/stuttgart-things/clusterscope/commit/595c6c63100f850e77056fb57a9234a57defe103))

## [0.24.0](https://github.com/stuttgart-things/clusterscope/compare/v0.23.1...v0.24.0) (2026-10-14)


### Features

* **lab:** multiple repos + cluster name/lab overrides (-config) ([#69](https://github.com/stuttgart-things/clusterscope/issues/69)) ([e14208c](https://github.com/stuttgart-things/clusterscope/commit/e14208c0a756b7a2498f625d7f5618d7bfe811bf)), closes [#68](https://github.com/stuttgart-things/clusterscope/issues/68)

## [0.23.1](https://github.com/stuttgart-things/clusterscope/compare/v0.23.0...v0.23.1) (2026-10-14)


### Bug Fixes

* **release:** pin the released image in the kustomize bases (release-please) ([#62](https://github.com/stuttgart-things/clusterscope/issues/62)) ([275a826](https://github.com/stuttgart-things/clusterscope/commit/275a8260b2ad8605546b02e0e1b0526f71448e53))
