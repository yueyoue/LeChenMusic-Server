import { SET_SELECTED_LIBRARIES, SET_USER_LIBRARIES } from '../actions'

const initialState = {
  userLibraries: [],
  selectedLibraries: [], // Empty means "all accessible libraries"
}

export const libraryReducer = (previousState = initialState, payload) => {
  const { type, data } = payload
  switch (type) {
    case SET_USER_LIBRARIES: {
      const newUserLibraryIds = data.map((lib) => lib.id)
      const previousUserLibraryIds = previousState.userLibraries.map(
        (lib) => lib.id,
      )

      // Validate and filter selected libraries to only include IDs that exist in new user libraries
      const validatedSelection = previousState.selectedLibraries.filter((id) =>
        newUserLibraryIds.includes(id),
      )
      // Newly granted libraries must join the selection. Otherwise a library added
      // after the user's first login (e.g. a cloud-drive library) stays out of the
      // saved selection forever and its songs are silently filtered out of every
      // list and search.
      const newlyGranted = newUserLibraryIds.filter(
        (id) => !previousUserLibraryIds.includes(id),
      )

      // Determine the final selection:
      // 1. If first time setting libraries (no previous user libraries), select all
      // 2. If user now has only one library, reset to empty (no filter needed)
      // 3. Otherwise, use validated selection plus any newly granted libraries
      let finalSelection
      if (
        previousState.selectedLibraries.length === 0 &&
        previousState.userLibraries.length === 0
      ) {
        // First time: select all libraries
        finalSelection = newUserLibraryIds
      } else if (newUserLibraryIds.length === 1) {
        // Single library: reset selection (empty means "all accessible")
        finalSelection = []
      } else {
        // Multiple libraries: validated selection + newly granted libraries
        finalSelection = [...new Set([...validatedSelection, ...newlyGranted])]
      }

      return {
        ...previousState,
        userLibraries: data,
        selectedLibraries: finalSelection,
      }
    }
    case SET_SELECTED_LIBRARIES:
      return {
        ...previousState,
        selectedLibraries: data,
      }
    default:
      return previousState
  }
}
