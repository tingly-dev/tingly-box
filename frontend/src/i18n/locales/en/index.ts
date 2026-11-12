// One file per feature area, each holding several top-level t() namespaces;
// see .design/i18n.md. Keep every locale's file set in step with en/.
import app from './app';
import audio from './audio';
import provider from './provider';
import rule from './rule';
import dashboard from './dashboard';
import bench from './bench';
import agent from './agent';
import remote from './remote';
import image from './image';
import claudeCode from './claudeCode';
import projects from './projects';

export default {
    ...app,
    ...audio,
    ...provider,
    ...rule,
    ...dashboard,
    ...bench,
    ...agent,
    ...remote,
    ...image,
    ...claudeCode,
    ...projects,
};
