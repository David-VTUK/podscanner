# Podscanner

A CLI tool, created initially as a training exercise, iterates through namespaces and identifies Pods with containers
using the `:latest` or no tag. 

# Usage

`podscanner` can be used with optional flags:

```bash
Flags:
  -h, --help                help for podscanner
  -k, --kubeconfig string   Optional: specify kubeconfig (if not defined, defaults to ~/.kube/config
  -n, --namespace string    Optional: specify namespace (if not defined, all namespaces will be scanned)
```

# Examples

```bash
podscanner                                                         

--namespace flag not used, scanning all namespces
┌─────────────┬──────────────────────────────┬───────────┬────────────────────────────────┐
│ NAMESPACE   │ POD                          │ CONTAINER │ IMAGE                          │
├─────────────┼──────────────────────────────┼───────────┼────────────────────────────────┤
│ kube-verify │ kube-verify-7464f954f4-l9wcr │ nginx     │ nginx                          │
├─────────────┼──────────────────────────────┼───────────┼────────────────────────────────┤
│ openfaas-fn │ curl-7c7795c5b8-bpb2t        │ curl      │ ghcr.io/openfaas/curl:latest   │
├─────────────┼──────────────────────────────┼───────────┼────────────────────────────────┤
│ openfaas-fn │ env-6dc77984d8-d4zdx         │ env       │ ghcr.io/openfaas/alpine:latest │
└─────────────┴──────────────────────────────┴───────────┴────────────────────────────────┘

```
```bash
podscanner --namespace openfaas-fn --kubeconfig ~/.kube/config                                                             1 ✘ 

--namespace flag used, only scanning openfaas-fn
┌─────────────┬───────────────────────┬───────────┬────────────────────────────────┐
│ NAMESPACE   │ POD                   │ CONTAINER │ IMAGE                          │
├─────────────┼───────────────────────┼───────────┼────────────────────────────────┤
│ openfaas-fn │ curl-7c7795c5b8-bpb2t │ curl      │ ghcr.io/openfaas/curl:latest   │
├─────────────┼───────────────────────┼───────────┼────────────────────────────────┤
│ openfaas-fn │ env-6dc77984d8-d4zdx  │ env       │ ghcr.io/openfaas/alpine:latest │
└─────────────┴───────────────────────┴───────────┴────────────────────────────────┘

```