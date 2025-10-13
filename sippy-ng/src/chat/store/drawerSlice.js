/**
 * Drawer slice - manages global chat drawer state
 * Handles drawer open/closed state only
 */
export const createDrawerSlice = (set, get) => ({
  // State
  isDrawerOpen: false,

  // Actions
  openDrawer: () => {
    set({ isDrawerOpen: true })
  },

  closeDrawer: () => {
    set({ isDrawerOpen: false })
  },

  toggleDrawer: () => {
    const { isDrawerOpen } = get()
    set({ isDrawerOpen: !isDrawerOpen })
  },
})
