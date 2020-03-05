module.exports = {
    title: 'Flagger',
    description: 'Progressive Delivery operator for Kubernetes (Canary, A/B Testing and Blue/Green deployments)',
    themeConfig: {
        search: true,
        activeHeaderLinks: false,
        docsDir: 'docs',
        repo: 'weaveworks/flagger',
        nav: [
            { text: 'Docs', link: 'https://docs.flagger.app' },
            { text: 'Changelog', link: 'https://github.com/weaveworks/flagger/blob/master/CHANGELOG.md' }
        ],
        sidebar: [
            '/',
            {
                title: 'Introduction',
                path: '/intro/',
                collapsable: false,
                children: [
                    ['/intro/', 'Get Started'],
                    ['/intro/faq', 'FAQ'],
                ],
            },
            {
                title: 'Install',
                path: '/install/flagger-install-on-kubernetes',
                collapsable: false,
                children: [
                    ['/install/flagger-install-on-kubernetes', 'On Kubernetes'],
                    ['/install/flagger-install-on-google-cloud', 'On GKE Istio'],
                    ['/install/flagger-install-on-eks-appmesh', 'On EKS App Mesh'],
                ],
            },
            {
                title: 'Usage',
                path: '/usage/how-it-works',
                collapsable: false,
                children: [
                    ['/usage/how-it-works', 'How it works'],
                    ['/usage/deployment-strategies', 'Deployment Strategies'],
                    ['/usage/metrics', 'Metrics Analysis'],
                    ['/usage/webhooks', 'Webhooks'],
                    ['/usage/alerting', 'Alerting'],
                    ['/usage/monitoring', 'Monitoring'],
                ],
            },
            {
                title: 'Tutorials',
                path: '/tutorials/istio-progressive-delivery',
                collapsable: false,
                children: [
                    ['/tutorials/istio-progressive-delivery', 'Istio Canaries'],
                    ['/tutorials/istio-ab-testing', 'Istio A/B Testing'],
                    ['/tutorials/linkerd-progressive-delivery', 'Linkerd Canaries'],
                    ['/tutorials/appmesh-progressive-delivery', 'App Mesh Canaries'],
                    ['/tutorials/nginx-progressive-delivery', 'NGINX Ingress Canaries'],
                    ['/tutorials/gloo-progressive-delivery', 'Gloo Canaries'],
                    ['/tutorials/contour-progressive-delivery', 'Contour Canaries'],
                    ['/tutorials/kubernetes-blue-green', 'Kubernetes Blue/Green'],
                    ['/tutorials/canary-helm-gitops', 'Canaries with Helm charts and GitOps'],
                ],
            },
            {
                title: 'Dev',
                path: '/dev/dev-guide',
                collapsable: false,
                children: [
                    ['/dev/dev-guide', 'Development Guide'],
                    ['/dev/release-guide', 'Release Guide'],
                    ['/dev/upgrade-guide', 'Upgrade Guide'],
                ],
            },
        ]
    },
    head: [
        ['link', { rel: 'icon', href: '/favicon.png' }],
        ['link', { rel: 'stylesheet', href: '/website.css' }],
        ['meta', { name: 'keywords', content: 'gitops kubernetes flagger istio linkerd appmesh contour gloo nginx' }],
        ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
        ['meta', { name: 'twitter:title', content: 'Flagger' }],
        ['meta', { name: 'twitter:description', content: 'Progressive delivery Kubernetes operator (Canary, A/B Testing and Blue/Green deployments)' }],
        ['meta', { name: 'twitter:image:src', content: 'https://flagger.app/flagger-overview.png' }]
    ]
};
