kubectl sigstore kustomize-build

## 必要なコマンド
- kubectl
- kustomize

- cosign
    ```
    $ git clone https://github.com/sigstore/cosign.git
    $ cd cosign
    $ make cosign
    $ sudo mv ./cosign /usr/local/bin/cosign
    ```

- rekor-cli
    ```
    $ git clone https://github.com/sigstore/rekor.git
    $ cd rekor
    $ make rekor-cli
    $ sudo mv ./rekor-cli /usr/local/bin/rekor-cli
    ```

- kubectl-sigstore
    ```
    $ git clone https://github.com/hirokuni-kitahara/k8s-manifest-sigstore.git
    $ cd k8s-manifest-sigstore
    $ git fetch origin
    $ git checkout feature/provenance
    $ make build
    $ sudo mv ./kubectl-sigstore /usr/local/bin/kubectl-sigstore
    ```

- genprov & gen-attestation
    ```
    $ git clone https://github.com/hirokuni-kitahara/misc.git
    $ cd misc/kustomize-build-poc
    $ make genprov
    $ make gen-attestation
    $ sudo mv ./genprov /usr/local/bin/genprov
    $ sudo mv ./gen-attestation /usr/local/bin/gen-attestation

    （簡単のために上記全てパスを通していますが、通さない場合は以下の実行コマンドを適宜読み替えてください）
    ```

## メインステップ
1. kustomize configリポをgit clone
    ```
    $ git clone https://github.com/hirokuni-kitahara/akmebank-config.git
    $ cd akmebank-config
    ```

2. cosign keyペア生成
    ```
    $ cosign generate-key-pair
    $ ls -l cosign.key cosign.pub
    -rw-------  1 user  staff  649 Jul 29 16:37 cosign.key
    -rw-------  1 user  staff  178 Jul 29 16:37 cosign.pub
    ```

3. manifestビルド
    ```
    $ kustomize build roles/dev/ > manifest.yaml
    $ cat manifest.yaml | grep -E "^kind:"
    kind: Service
    kind: Service
    kind: Service
    kind: Deployment
    kind: Deployment
    kind: Deployment
    kind: Route
    ```

4. provenance.json生成
    ```
    $ genprov roles/dev/ manifest.yaml > provenance.json
    $ cat provenance.json | jq . | head 
    {
    "_type": "https://in-toto.io/Statement/v0.1",
    "predicateType": "https://in-toto.io/Provenance/v0.1",
    "subject": [
        {
        "name": "manifest.yaml",
        "digest": {
            "sha256": "c59ae079fc5f73f4ba2acdf20946a93baba98ec19a70854144c150d6104a5407"
        }
        }
    ```

5. create & push manifest image
    ```
    $ cosign upload blob -f manifest.yaml gcr.io/hk-image-registry/sample-manifest:dev
    $ cosign sign -key cosign.key gcr.io/hk-image-registry/sample-manifest:dev
    ```

6. generate attestation.json
    ```
    $ gen-attestation provenance.json cosign.key gcr.io/hk-image-registry/sample-manifest:dev > attestation.json
    $ cat attestation.json | jq . 
    {
    "payloadType": "application/vnd.in-toto+json",
    "payload": "eyJfdHl ... 19fQ==",
    "signatures": [
        {
        "keyid": "",
        "sig": "MEYC ... uWapz"
        }
    ]
    }
    ```

7. upload attestation tlog
    ```
    $ rekor-cli upload --artifact ./attestation.json --public-key cosign.pub --type intoto --pki-format x509
    Created entry at index 26967, available at: https://rekor.sigs ....
    ```


### 確認ステップ

8. deploy a manifest.yaml
    ```
    $ kubectl create ns custom-ns
    $ kubectl create -f manifest.yaml
    $ kubectl get all -n custom-ns
    NAME                                      READY   STATUS              RESTARTS   AGE
    pod/akme-account-command-78974d78-ngws9   0/1     ContainerCreating   0          7m43s
    pod/akme-account-query-56854785f-8kjj5    0/1     ContainerCreating   0          7m43s
    pod/akme-akmebank-ui-6bc44784c-nz7g6      1/1     Running             0          7m43s

    ...

    (ContainerCreatingのPodをRunningにするには、custom-nsにSecret "cos-secret" をつくる)
    (以下の9.は "cos-secret" なしのときの例)
    ```

9. verify-resource
    ```
    $ kubectl sigstore verify-resource -n custom-ns -i gcr.io/hk-image-registry/sample-manifest:dev -k cosign.pub --provenance

    (上の方は割愛)

    [RESOURCES - PODS/CONTAINERS]
    POD                                CONTAINER     IMAGE ID                                                                                                       ATTESTATION   SBOM
    akme-akmebank-ui-6bc44784c-nz7g6   akmebank-ui   gcr.io/hk-image-registry/akmebank-ui@sha256:b205a834c1a3ee6687b4f62193bb58a071cf77b2c573d106722d7dd928840def   found         found

    [PROVENANCES - ATTESTATIONS]
    ARTIFACT               gcr.io/hk-image-registry/sample-manifest:dev
    MATERIALS   URI        roles/dev/kustomization.yaml
                HASH       2f8c7e60bad0b1e14492aab9dbc7d5cbc710007b7f9777d960d0564b63afb47a
                URI        https://github.com/gajananan/akmebank-app.git
                COMMIT     bcea6772ff35dc3004d84f6474c02315e5d8141c
                PATH       deploy/base
                REVISION   master
    To get this attestation: curl -s "https://rekor.sigstore.dev/api/v1/log/entries/?logIndex=26967"

    ARTIFACT               gcr.io/hk-image-registry/akmebank-ui:4.2.1
    MATERIALS   URI        https://github.com/gajananan/akmebank-app.git
                COMMIT     bcea6772ff35dc3004d84f6474c02315e5d8141c
                REVISION   master
    To get this attestation: curl -s "https://rekor.sigstore.dev/api/v1/log/entries/?logIndex=7167"


    [PROVENANCES - SBOMs]
    ARTIFACT    gcr.io/hk-image-registry/akmebank-ui:4.2.1
    SBOM NAME   gcr.io/hk-image-registry/akmebank-ui:sha256-b205a834c1a3ee6687b4f62193bb58a071cf77b2c573d106722d7dd928840def.sbom
    To download SBOM: cosign download sbom gcr.io/hk-image-registry/akmebank-ui:4.2.1
    ```

