# multiclusterhub Configurations

This directory contains examples that cover various configurations for multiclusterhub.

### Custom pull secret

```yaml
spec:
  imagePullSecret: "quay-secret"
```

### HA installation with node selector

```yaml
spec:
  failover: true
  nodeSelector:
      diskType: ssd
```

### Specify ingress SSL ciphers to support

```yaml
spec:
  ingress:
    sslCiphers:
    - "ECDHE-ECDSA-AES128-GCM-SHA256"
    - "ECDHE-RSA-AES128-GCM-SHA256"
```

### Install Cert Manager in its own namespace

```yaml
spec:
  separateCertificateManagement: true
```

### Specific image pull policy:

```yaml
spec:
  overrides: true
    imagePullPolicy: "IfNotPresent"
```

## Dev Configurations

### Custom image repository and tag suffix

```yaml
apiVersion: operator.open-cluster-management.io/v1
kind: MultiClusterHub
metadata:
  name: multiclusterhub
  namespace: open-cluster-management
  annotations:
    "mch-imageRepository": "quay.io/open-cluster-management"
    "mch-imageTagSuffix": "SNAPSHOT-2020-06-18-13-43-50"
```

### Disable install operator actions

```yaml
apiVersion: operator.open-cluster-management.io/v1
kind: MultiClusterHub
metadata:
  name: multiclusterhub
  namespace: open-cluster-management
  annotations:
    "mch-pause": "true"
```