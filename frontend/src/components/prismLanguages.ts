// Languages prism-react-renderer doesn't bundle but that code blocks commonly
// hold (shell commands and diffs above all). Import order matters: the global
// must exist before each language file is evaluated.
import './prismGlobal';
import 'prismjs/components/prism-bash';
import 'prismjs/components/prism-diff';
import 'prismjs/components/prism-docker';
import 'prismjs/components/prism-toml';
import 'prismjs/components/prism-ini';
import 'prismjs/components/prism-java';
import 'prismjs/components/prism-powershell';
