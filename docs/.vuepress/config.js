module.exports = {
    title: 'Flagger',
    description: 'Progressive Delivery operator for Kubernetes (Canary, A/B Testing and Blue/Green deployments)',
    themeConfig: {
        search: false,
        activeHeaderLinks: false,
        repo: 'weaveworks/flagger',
        nav: [
            { text: 'Docs', link: 'https://docs.flagger.app' },
            { text: 'Changelog', link: 'https://github.com/weaveworks/flagger/blob/master/CHANGELOG.md' }
        ]
    },
    head: [
        ['link', { rel: 'icon', href: '/favicon.png' }],
        ['link', { rel: 'stylesheet', href: '/website.css' }],
        ['meta', { name: 'keywords', content: 'gitops kubernetes flagger istio linkerd appmesh' }],
        ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
        ['meta', { name: 'twitter:title', content: 'Flagger' }],
        ['meta', { name: 'twitter:description', content: 'Progressive delivery Kubernetes operator (Canary, A/B Testing and Blue/Green deployments)' }],
        ['meta', { name: 'twitter:image:src', content: 'https://flagger.app/flagger-overview.png' }]
    ]
};
