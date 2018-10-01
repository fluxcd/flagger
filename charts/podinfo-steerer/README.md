# Podinfo Istio

Podinfo is a tiny web application made with Go 
that showcases best practices of running microservices in Kubernetes.

## Installing the Chart

Create an Istio enabled namespace:

```console
kubectl create namespace test
kubectl label namespace test istio-injection=enabled
```

Create an Istio Gateway in the `istio-system` namespace named `public-gateway`:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: Gateway
metadata:
  name: public-gateway
  namespace: istio-system
spec:
  selector:
    istio: ingressgateway
  servers:
  - port:
      number: 80
      name: http
      protocol: HTTP
    hosts:
    - "*"
    tls:
      httpsRedirect: true
  - port:
      number: 443
      name: https
      protocol: HTTPS
    hosts:
    - "*"
    tls:
      mode: SIMPLE
      privateKey: /etc/istio/ingressgateway-certs/tls.key
      serverCertificate: /etc/istio/ingressgateway-certs/tls.crt
```

Create the `frontend` release by specifying the external domain name:

```console
helm upgrade frontend -i ./charts/podinfo-steerer \
  --namespace=test \
  --set gateway.enabled=true \
  --set gateway.host=podinfo.example.com 
```


