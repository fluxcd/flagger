/**
 * Common module constructor.
 */
export declare class CommonModule {
    name: string | null;
    entry: string | null;
    shortcut: string | null;
    fromDep: boolean | null;
    constructor(entry: string | null, name: string | null, shortcut: string | null, fromDep: boolean | null);
}
export interface NormalizedModuleRequest {
    name: string | null;
    shortcut: string | null;
}
/**
 * Expose ModuleResolver.
 */
declare type Type = String | Number | Boolean | RegExp | Function | Object | Record<string, any> | Array<any>;
declare class ModuleResolver {
    private type;
    private org;
    private allowedTypes;
    private load;
    private cwd;
    private nonScopePrefix;
    private scopePrefix;
    private typePrefixLength;
    private prefixSlicePosition;
    constructor(type: string, org: string, allowedTypes: Type[] | undefined, load: boolean | undefined, cwd: string);
    /**
     * Resolve package.
     */
    resolve(req: string, cwd: string): CommonModule | never;
    /**
     * Set current working directory.
     */
    private setCwd;
    /**
     * Resolve non-string package, return directly.
     */
    private resolveNonStringPackage;
    /**
     * Resolve module with absolute path.
     */
    resolveAbsolutePathPackage(req: string): CommonModule;
    /**
     * Resolve module with absolute path.
     */
    private resolveRelativePathPackage;
    /**
     * Resolve module from dependency.
     */
    private resolveDepPackage;
    /**
     * Get shortcut.
     */
    private getShortcut;
    /**
     * Normalize string request name.
     */
    normalizeName(req: string): NormalizedModuleRequest;
    /**
     * Normalize any request.
     */
    normalizeRequest(req: any): NormalizedModuleRequest;
}
/**
 * Parse info of scope package.
 */
export interface ScopePackage {
    org: string;
    name: string;
}
export declare function resolveScopePackage(name: string): {
    org: string;
    name: string;
};
export declare const getMarkdownItResolver: (cwd: string) => ModuleResolver;
export declare const getPluginResolver: (cwd: string) => ModuleResolver;
export declare const getThemeResolver: (cwd: string) => ModuleResolver;
export {};
