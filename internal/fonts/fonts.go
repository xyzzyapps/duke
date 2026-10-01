// Package fonts embeds the runtime TrueType font. JetBrains Mono is the
// coding monospace used for both the document and chrome faces; it is
// licensed under the SIL Open Font License 1.1 (see assets/OFL.txt).
package fonts

import _ "embed"

// JetBrainsMonoTTF is the embedded TrueType data (OFL 1.1).
//
//go:embed assets/JetBrainsMono-Regular.ttf
var JetBrainsMonoTTF []byte
