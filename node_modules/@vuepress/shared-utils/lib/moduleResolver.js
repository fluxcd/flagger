'use strict';
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
/**
 * Module dependencies.
 */
const upath_1 = __importDefault(require("upath"));
const chalk_1 = __importDefault(require("chalk"));
const moduleLoader_1 = require("./moduleLoader");
const tryChain_1 = __importDefault(require("./tryChain"));
const fallback_1 = require("./fallback");
const hash_sum_1 = __importDefault(require("hash-sum"));
const datatypes_1 = require("./datatypes");
const SCOPE_PACKAGE_RE = /^@(.*)\/(.*)/;
/**
 * Common module constructor.
 */
class CommonModule {
    constructor(entry, name, shortcut, fromDep) {
        this.entry = entry;
        this.shortcut = shortcut;
        this.name = name;
        this.fromDep = fromDep;
    }
}
exports.CommonModule = CommonModule;
class ModuleResolver {
    constructor(type, org, allowedTypes = [String], load = false, cwd) {
        this.type = type;
        this.org = org;
        this.allowedTypes = allowedTypes;
        this.load = load;
        this.cwd = cwd || process.cwd();
        if (org) {
            this.nonScopePrefix = `${org}-${type}-`;
            this.scopePrefix = `@${org}/${type}-`;
        }
        else {
            this.nonScopePrefix = `${type}-`;
        }
        this.typePrefixLength = type.length + 1;
        /* - */
        this.prefixSlicePosition = this.typePrefixLength + org.length + 1;
        /* @ */
    }
    /**
     * Resolve package.
     */
    resolve(req, cwd) {
        if (cwd) {
            this.setCwd(cwd);
        }
        const { valid, warnMsg } = datatypes_1.assertTypes(req, this.allowedTypes);
        if (!valid) {
            throw new Error(`Invalid value for "${chalk_1.default.cyan(this.type)}": ${warnMsg}`);
        }
        const isStringRequest = datatypes_1.isString(req);
        const isAbsolutePath = isStringRequest && upath_1.default.isAbsolute(req);
        const resolved = tryChain_1.default([
            [this.resolveNonStringPackage.bind(this), !isStringRequest],
            [this.resolveAbsolutePathPackage.bind(this), isStringRequest && isAbsolutePath],
            [this.resolveRelativePathPackage.bind(this), isStringRequest && !isAbsolutePath],
            [this.resolveDepPackage.bind(this), isStringRequest]
        ], req);
        if (!resolved) {
            return new CommonModule(null, null, null, null /* fromDep */);
        }
        return resolved;
    }
    /**
     * Set current working directory.
     */
    setCwd(cwd) {
        this.cwd = cwd;
        return this;
    }
    /**
     * Resolve non-string package, return directly.
     */
    resolveNonStringPackage(req) {
        const { shortcut, name } = this.normalizeRequest(req);
        return new CommonModule(req, name, shortcut, false /* fromDep */);
    }
    /**
     * Resolve module with absolute path.
     */
    resolveAbsolutePathPackage(req) {
        const normalized = fallback_1.fsExistsFallback([
            req,
            req + '.js',
            upath_1.default.resolve(req, 'index.js')
        ]);
        if (!normalized) {
            throw new Error(`${req} Not Found.`);
        }
        const dirname = upath_1.default.parse(normalized).name;
        const { shortcut, name } = this.normalizeRequest(dirname);
        const module = this.load ? require(normalized) : normalized;
        return new CommonModule(module, name, shortcut, false /* fromDep */);
    }
    /**
     * Resolve module with absolute path.
     */
    resolveRelativePathPackage(req) {
        req = upath_1.default.resolve(process.cwd(), req);
        return this.resolveAbsolutePathPackage(req);
    }
    /**
     * Resolve module from dependency.
     */
    resolveDepPackage(req) {
        const { shortcut, name } = this.normalizeRequest(req);
        const entry = this.load
            ? moduleLoader_1.loadModule(name, this.cwd)
            : moduleLoader_1.resolveModule(name, this.cwd);
        return new CommonModule(entry, name, shortcut, true /* fromDep */);
    }
    /**
     * Get shortcut.
     */
    getShortcut(req) {
        return req.startsWith(this.nonScopePrefix)
            ? req.slice(this.prefixSlicePosition)
            : req;
    }
    /**
     * Normalize string request name.
     */
    normalizeName(req) {
        let name;
        let shortcut;
        if (req.startsWith('@')) {
            const pkg = resolveScopePackage(req);
            if (pkg) {
                // speicial handling for default org.
                if (this.org && pkg.org === this.org) {
                    shortcut = pkg.name.startsWith(`${this.type}-`)
                        ? pkg.name.slice(this.typePrefixLength)
                        : pkg.name;
                    name = `${this.scopePrefix}${shortcut}`;
                }
                else {
                    shortcut = this.getShortcut(pkg.name);
                    name = `@${pkg.org}/${this.nonScopePrefix}${shortcut}`;
                }
                shortcut = `@${pkg.org}/${shortcut}`;
            }
        }
        else {
            shortcut = this.getShortcut(req);
            name = `${this.nonScopePrefix}${shortcut}`;
        }
        // @ts-ignore
        return { name, shortcut };
    }
    /**
     * Normalize any request.
     */
    normalizeRequest(req) {
        if (datatypes_1.isString(req)) {
            return this.normalizeName(req);
        }
        if (datatypes_1.isObject(req) || datatypes_1.isFunction(req)) {
            if (datatypes_1.isString(req.name)) {
                return this.normalizeName(req.name);
            }
            else {
                const shortcut = `anonymous-${hash_sum_1.default(req)}`;
                const name = `${this.nonScopePrefix}${shortcut}`;
                return { name, shortcut };
            }
        }
        return {
            name: null,
            shortcut: null
        };
    }
}
function resolveScopePackage(name) {
    if (SCOPE_PACKAGE_RE.test(name)) {
        return {
            org: RegExp.$1,
            name: RegExp.$2
        };
    }
    return {
        org: '',
        name: ''
    };
}
exports.resolveScopePackage = resolveScopePackage;
exports.getMarkdownItResolver = (cwd) => new ModuleResolver('markdown-it', '', [String, Function], true /* load module */, cwd);
exports.getPluginResolver = (cwd) => new ModuleResolver('plugin', 'vuepress', [String, Function, Object], true /* load module */, cwd);
exports.getThemeResolver = (cwd) => new ModuleResolver('theme', 'vuepress', [String], false /* load module */, cwd);
