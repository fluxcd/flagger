apiVersion: templates.gatekeeper.sh/v1beta1
kind: ConstraintTemplate
metadata:
  name: canaryAdmission
spec:
  crd:
    spec:
      names:
        kind: canaryAdmission
        listKind: canaryAdmissionList
        plural: canaryAdmission
        singular: canaryAdmission
      validation:
        # Schema for the `canaryAnalysis` field
        openAPIV3Schema:
          properties:
            interval:
              description: Schedule interval for this canary
              type: string
              pattern: "^[0-9]+(m|s)"
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package canaryAdmission

        default allow = false       

        allow = true {              
            count(violation) == 0
            count(valid_target) == 0
        }

        valid_target = [target | 
          target := input.spec.targetRef
            valid_match(target)
        ]

        valid_match(target) {
          target.kind != "Deployment"
        }

        valid_match(target) {
          count(target.name) < 1
        }

        violation[canary] {
          canary := input.spec.canaryAnalysis
          canary.stepWeight > 0
            canary.iterations > 0
        }

        violation[canary] {
          canary := input.spec.canaryAnalysis
          count(canary.match) > 0
            canary.iterations == 0
        }

        violation[canary] {
          canary := input.spec.canaryAnalysis
            canary.stepWeight > canary.maxWeight
        }

        violation[canary] {
          canary := input.spec.canaryAnalysis
            canary.stepWeight > 100
        }

        violation[canary] {
          canary := input.spec.canaryAnalysis
            canary.maxWeight > 100
        } 

        violation[spec] {
          spec := input.spec
          provider := spec.provider
            service := spec.service
            provider == "appmesh"
            count(service.meshName) < 1
        }

        violation[spec] {
          spec := input.spec
          spec.progressDeadlineSeconds > 0
            spec.progressDeadlineSeconds < 60
        }

        violation[canary] {
          canary := input.spec.canaryAnalysis
            metric := canary.metrics[_]
            metric.name != "request-success-rate"
            metric.name != "request-duration"
          count(metric.query) < 1
        }

        # violation[ingressRef] {
        # 	ingressRef := input.spec.ingressRef
        #     ingressRef
        # 	# ingressRef != nil && (ingressRef.kind != "Ingress" || len(ingressRef.name) < 1)
        # }

        # fix this - issue with M/S
        # violation[canary] {
        # 	canary := input.spec.canaryAnalysis
        #     re_match("^[0-9]+(m|s)", canary.interval)
        #     output := regex.split("(m|s)", canary.interval)
        #     t := to_number(output[0])
        #     t < 10
        # }

        # fix this - issue with M/S
        # violation[canary] {
        # 	canary := input.spec.canaryAnalysis
        # 	metric := canary.metrics[_]
        #     re_match("^[0-9]+(m|s)", metric.interval)
        #     output := regex.split("(m|s)", metric.interval)
        #     t := to_number(output[0])   
        # 	t < 10
        # }

        # fix this - none of the regex i use works
        # violation[canary] {
        # 	canary := input.spec.canaryAnalysis
        #     webhook := canary.webhooks[_]
        #     re_match("", webhook.url)
        # }

        # Missing
        # autoscalerRef != nil && (autoscalerRef.kind != "HorizontalPodAutoscaler" || len(autoscalerRef.name) < 1)

