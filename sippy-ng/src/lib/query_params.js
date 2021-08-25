import JSONCrush from 'jsoncrush'

// CompressedJsonParam is used as a query parameter type that is in JSON, and
// compressed using JSONCrush. This should make the URL's shared by Sippy less
// ugly, but there's really only so much we can do with a stateless app that needs
// shareable URL's with complex filtering.
export const CompressedJsonParam = {
  encode: (param) => {
    if (param === null) {
      return param
    }

    return JSONCrush.crush(JSON.stringify(param))
  },
  decode: (param) => {
    if (!param) {
      return param
    }

    // Backwards compatability with uncompressed JSON URL's that were previously shared
    if (param.charAt(0) === '{') {
      return JSON.parse(param)
    } else {
      return JSON.parse(JSONCrush.uncrush(param))
    }
  }
}
