```
1. kubectl create ns custom-ns

2. kubectl apply -f kustomize-built-akmebank-app-manifest.yaml

  (the manifest yaml above is built by kustomize with https://github.com/hirokuni-kitahara/akmebank-config/blob/main/roles/dev/kustomization.yaml)
  (`Route` object cannot be created in Kind cluster until CRD is registered)


3. kubectl sigstore verify-resource -n custom-ns -i gcr.io/hk-image-registry/akmebank-app-manifest:1.0.0 -k cosign.pub --provenance -o json
```
