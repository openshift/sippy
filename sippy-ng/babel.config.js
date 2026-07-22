function importMetaEnvPlugin({ types: t }) {
  return {
    visitor: {
      MemberExpression(path) {
        if (
          path.get('object').isMemberExpression() &&
          path.get('object.object').isMetaProperty() &&
          path.get('object.property').isIdentifier({ name: 'env' })
        ) {
          const envKey = path.get('property').node.name
          if (envKey === 'PROD') {
            path.replaceWith(
              t.binaryExpression(
                '===',
                t.memberExpression(
                  t.memberExpression(
                    t.identifier('process'),
                    t.identifier('env')
                  ),
                  t.identifier('NODE_ENV')
                ),
                t.stringLiteral('production')
              )
            )
          } else if (envKey === 'DEV') {
            path.replaceWith(
              t.binaryExpression(
                '!==',
                t.memberExpression(
                  t.memberExpression(
                    t.identifier('process'),
                    t.identifier('env')
                  ),
                  t.identifier('NODE_ENV')
                ),
                t.stringLiteral('production')
              )
            )
          } else {
            path.replaceWith(
              t.memberExpression(
                t.memberExpression(
                  t.identifier('process'),
                  t.identifier('env')
                ),
                t.identifier(envKey)
              )
            )
          }
        }
      },
    },
  }
}

module.exports = {
  presets: [
    ['@babel/preset-env', { targets: { node: 'current' } }],
    ['@babel/preset-react', { runtime: 'classic' }],
  ],
  plugins: [importMetaEnvPlugin],
}
