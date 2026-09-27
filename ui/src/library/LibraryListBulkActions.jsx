import React from 'react'
import LibraryScanButton from './LibraryScanButton'
import DeleteLibrariesButton from './DeleteLibrariesButton'

const LibraryListBulkActions = (props) => (
  <>
    <LibraryScanButton fullScan={false} {...props} />
    <LibraryScanButton fullScan={true} {...props} />
    <DeleteLibrariesButton />
  </>
)

export default LibraryListBulkActions
