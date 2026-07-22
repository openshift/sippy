module.exports = {
  testEnvironment: 'jsdom',
  setupFilesAfterEnv: ['<rootDir>/src/setupTests.jsx'],
  transform: {
    '^.+\\.jsx?$': 'babel-jest',
  },
  moduleNameMapper: {
    '\\.(css)$': 'identity-obj-proxy',
    '\\.(svg|png|jpg|jpeg|gif|ico|webp)$':
      '<rootDir>/src/__mocks__/fileMock.jsx',
  },
  snapshotSerializers: ['jest-serializer-html'],
}
