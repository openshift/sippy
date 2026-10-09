export class GridView {
  constructor(columns, views, defaultView) {
    this.allColumns = columns
    this.filterColumns = Object.entries(columns).map(([_, v]) => v)
    this.views = views
    this.columnVisibilityModel = {}
    this.setView(defaultView)
  }

  setView(name) {
    if (name in this.views) {
      let columns = []
      const visibilityModel = {}
      this.view = this.views[name]
      this.viewName = name
      this.view.fieldOrder.forEach((e) => {
        let field = this.allColumns[e.field]
        if (field === undefined) {
          console.error(e.field + ' field not found')
        }
        if (e.hide) {
          visibilityModel[e.field] = false
        }
        field.flex = e.flex
        field.headerClassName = e.headerClassName
        columns.push(field)
      })
      this.columnVisibilityModel = visibilityModel
      this.columns = columns
    } else {
      console.error(name + ' is not a known view')
    }
  }
}
