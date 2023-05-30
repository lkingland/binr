# ARCHITECTURE

When the `Get` method is invoked, the system will consult the local
filesystem to check if the named command is available.  Commands are
namespaced, such that different client apps can have binaries of the same
name but are different.

On disk the commands themselves are content-addressed and symlinked, such that
caching spans all clients and namespaces.

The provided Source when calling Get is implemented by the caller, providing
the URLs which the system uses to download and validate.

... Work in Progress
