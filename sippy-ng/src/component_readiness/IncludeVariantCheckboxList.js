import { CompReadyVarsContext } from './CompReadyVars'
import CheckBoxList from './CheckboxList'
import PropTypes from 'prop-types'
import React, { useContext } from 'react'

export default function IncludeVariantCheckBoxList(props) {
  const variantName = props.variantName
  const setCheckedItems = (checkedItems) => {
    varsContext.replaceIncludeVariantsCheckedItems(variantName, checkedItems)
  }
  const varsContext = useContext(CompReadyVarsContext)
  return (
    <CheckBoxList
      headerName={'Include ' + variantName}
      displayList={varsContext.allJobVariants[variantName]}
      checkedItems={
        variantName in varsContext.includeVariantsCheckedItems
          ? varsContext.includeVariantsCheckedItems[variantName]
          : []
      }
      setCheckedItems={setCheckedItems}
    />
  )
}

IncludeVariantCheckBoxList.propTypes = {
  variantName: PropTypes.string,
}
