## @vue/babel-preset-jsx

Configurable preset for Vue JSX plugins.

### Babel Compatibility Notes

- This repo is only compatible with Babel 7.x, for 6.x please use [vuejs/babel-plugin-transform-vue-jsx](https://github.com/vuejs/babel-plugin-transform-vue-jsx)

### Usage

Install the dependencies:

```sh
# for yarn:
yarn add @vue/babel-preset-jsx @vue/babel-helper-vue-jsx-merge-props
# for npm:
npm install @vue/babel-preset-jsx @vue/babel-helper-vue-jsx-merge-props --save
```

In your `.babelrc`:

```json
{
  "presets": ["@vue/babel-preset-jsx"]
}
```

You can toggle specific features, by default all features are enabled, e.g.:

```json
{
  "presets": [
    [
      "@vue/babel-preset-jsx",
      {
        "vModel": false
      }
    ]
  ]
}
```

Options are:

- `functional` [@vue/babel-sugar-functional-vue](../babel-sugar-functional-vue/README.md) - Functional components syntactic sugar
- `injectH` [@vue/babel-sugar-inject-h](../babel-sugar-inject-h/README.md) - Automatic `h` injection syntactic sugar
- `vModel` [@vue/babel-sugar-v-model](../babel-sugar-v-model/README.md) - `vModel` syntactic sugar
- `vOn` [@vue/babel-sugar-v-on](../babel-sugar-v-on/README.md) - `vOn` syntactic sugar
