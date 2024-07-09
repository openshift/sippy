import { CompReadyVarsContext } from './CompReadyVars'
import CheckBoxList from './CheckboxList'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'

export default function IncludeVariantCheckBoxList(props) {
  const variantName = props.variantName
  const varsContext = useContext(CompReadyVarsContext)
  const [checkedItems, setCheckedItems] = React.useState(
    variantName in varsContext.includeVariantsCheckedItems
      ? varsContext.includeVariantsCheckedItems[variantName]
      : []
  )

  const updateCheckedItems = (newCheckedItems) => {
    varsContext.replaceIncludeVariantsCheckedItems(variantName, newCheckedItems)
    setCheckedItems(newCheckedItems)
  }
  return (
    <CheckBoxList
      headerName={'Include ' + variantName}
      displayList={varsContext.allJobVariants[variantName]}
      checkedItems={checkedItems}
      setCheckedItems={updateCheckedItems}
    />
  )
}

IncludeVariantCheckBoxList.propTypes = {
  variantName: PropTypes.string,
}
