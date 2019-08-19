module.exports = {
    title: 'Flagger',
    description: 'Progressive Delivery operator for Kubernetes',
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
        ['link', { rel: 'stylesheet', href: '/website.css' }]
    ]
};
