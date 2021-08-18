import { Button, Container, Divider, Typography } from '@material-ui/core'
import Dialog from '@material-ui/core/Dialog'
import DialogContent from '@material-ui/core/DialogContent'
import DialogTitle from '@material-ui/core/DialogTitle'
import { Close } from '@material-ui/icons'
import PropTypes from 'prop-types'
import React, { Fragment } from 'react'
import TestTable from '../tests/TestTable'

export default function JobDetailTestModal (props) {
  const filterModel = {
    linkOperator: 'or',
    items: []
  }

  if (props.item.failedTestNames) {
    props.item.failedTestNames.forEach((test, index) => {
      filterModel.items.push({
        id: index, columnField: 'name', operatorValue: 'equals', value: test
      })
    })
  }

  return (
        <Fragment>
            <Dialog
                scroll="paper"
                style={{ verticalAlign: 'top' }}
                maxWidth="md"
                fullWidth={true}
                open={props.isOpen}
                onClose={props.close}
                aria-labelledby="form-dialog-title">

                <DialogTitle id="form-dialog-title" style={{ textAlign: 'right' }}>
                    <Button startIcon={<Close />} onClick={props.close} />
                </DialogTitle>
                <DialogContent>
                    <Container size="xl">
                        <Typography variant="h4">
                            {props.item.name}
                        </Typography>
                        <Divider />
                        <Button target="_blank" href={props.item.prowLink} variant="contained" color="primary" style={{ marginTop: 20, marginBottom: 20 }}>
                            Open Prow Link
                        </Button>

                        <Typography variant="h5">
                            Failed tests from this run
                        </Typography>
                    </Container>

                    { filterModel.items.length > 0 ? <TestTable release={props.release} hideControls={true} filterModel={filterModel} /> : <Container size="xl"><p>No failed tests found.</p></Container> }
                </DialogContent>
            </Dialog>
        </Fragment >
  )
}

JobDetailTestModal.defaultProps = {
  item: {
    name: '',
    failedTestNames: [],
    prowLink: ''
  },
  classes: {}
}

JobDetailTestModal.propTypes = {
  item: PropTypes.array.object,
  classes: PropTypes.object,
  isOpen: PropTypes.bool,
  close: PropTypes.func,
  release: PropTypes.string.isRequired
}
