import {Prism} from 'prism-react-renderer';

// prismjs's language files register themselves on a global `Prism` when they
// are evaluated, so prism-react-renderer's instance has to be exposed before
// prismLanguages.ts imports them.
(globalThis as {Prism?: typeof Prism}).Prism = Prism;
