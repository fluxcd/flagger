const path = require('path')

module.exports = {
  snapshotSerializers: [
    require.resolve('jest-serializer-vue'),
  ],
}
