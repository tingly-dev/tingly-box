#ifndef TINGLY_VZ_NAMESPACE_H
#define TINGLY_VZ_NAMESPACE_H

// go-vz v3.8.0 and Wails both define AppDelegate. Force-include this header
// during GUI builds, but rename the class only in go-vz's Objective-C sources:
// its private headers are on that package's include path, not Wails'.
// Remove this workaround once upstream go-vz namespaces its delegate.
#if defined(__APPLE__) && defined(__OBJC__)
#if __has_include("virtualization_view.h") && __has_include("virtualization_helper.h")
#define AppDelegate TinglyVZAppDelegate
#endif
#endif

#endif
