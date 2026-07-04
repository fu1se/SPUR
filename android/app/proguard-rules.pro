# gomobile's generated Java/Kotlin glue (go.Seq et al., plus every class
# under the bound package spurmobile) is wired up to the native Go side
# via explicit JNI RegisterNatives calls that reference Java method names
# as plain strings — not through reflection R8 could trace, so it can't
# tell those names are load-bearing. Renaming or stripping them at build
# time leaves the native side looking up a method that no longer exists
# under that name, failing at runtime with NoSuchMethodError instead of
# at compile time. Keep both packages untouched rather than trying to
# enumerate the individual entry points gobind happens to generate today.
-keep class go.** { *; }
-keep class spurmobile.** { *; }
