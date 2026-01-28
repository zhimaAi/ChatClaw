//go:build linux && cgo && !android

package webviewpanel

/*
#cgo linux pkg-config: gtk+-3.0 gdk-3.0

#include <gtk/gtk.h>
#include <string.h>
#include <stdlib.h>

static gboolean _wvpanel_title_matches(GtkWindow *w, const char *title, gboolean contains) {
    const char *wt = gtk_window_get_title(w);
    if (wt == NULL || title == NULL) return FALSE;
    if (!contains) {
        return strcmp(wt, title) == 0;
    }
    return strstr(wt, title) != NULL;
}

static void* _wvpanel_find_window(const char *title, gboolean contains) {
    if (title == NULL) return NULL;
    // Ensure GTK is initialised. If Wails already initialised it, this is a no-op.
    if (!gtk_init_check(0, NULL)) {
        // Still try enumerating; but likely won't work
    }
    GList *wins = gtk_window_list_toplevels();
    for (GList *l = wins; l != NULL; l = l->next) {
        GtkWidget *ww = GTK_WIDGET(l->data);
        if (!GTK_IS_WINDOW(ww)) continue;
        GtkWindow *w = GTK_WINDOW(ww);
        if (_wvpanel_title_matches(w, title, contains)) {
            g_list_free(wins);
            return (void*)w;
        }
    }
    g_list_free(wins);
    return NULL;
}
*/
import "C"

import "unsafe"

// FindWindowByTitle finds a GtkWindow by its title and returns it as uintptr.
func FindWindowByTitle(title string) uintptr {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	return uintptr(C._wvpanel_find_window(ct, C.FALSE))
}

// FindWindowByTitleContains finds a GtkWindow whose title contains the given substring.
func FindWindowByTitleContains(titleSubstring string) uintptr {
	ct := C.CString(titleSubstring)
	defer C.free(unsafe.Pointer(ct))
	return uintptr(C._wvpanel_find_window(ct, C.TRUE))
}

