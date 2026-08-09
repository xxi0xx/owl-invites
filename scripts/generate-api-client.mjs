import { readFile, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const specPath = resolve(root, 'api/openapi.json');
const outputPath = resolve(root, 'web/src/lib/api/generated.ts');
const spec = JSON.parse(await readFile(specPath, 'utf8'));

function typeOf(schema, indent = '') {
	if (!schema) return 'void';
	if (schema.$ref) return schema.$ref.split('/').at(-1);
	if (schema.enum) return schema.enum.map((value) => JSON.stringify(value)).join(' | ');
	if (schema.oneOf) return schema.oneOf.map((entry) => typeOf(entry, indent)).join(' | ');
	if (schema.type === 'array') return `Array<${typeOf(schema.items, indent)}>`;
	if (schema.type === 'object' || schema.properties) {
		if (!schema.properties) return schema.additionalProperties ? 'Record<string, unknown>' : 'Record<string, never>';
		const required = new Set(schema.required || []);
		const fields = Object.entries(schema.properties).map(([name, value]) =>
			`${indent}\t${JSON.stringify(name)}${required.has(name) ? '' : '?'}: ${typeOf(value, indent + '\t')};`
		);
		return `{\n${fields.join('\n')}\n${indent}}`;
	}
	if (schema.type === 'integer' || schema.type === 'number') return 'number';
	if (schema.type === 'boolean') return 'boolean';
	if (schema.type === 'null') return 'null';
	return 'string';
}

function responseSchema(operation) {
	for (const code of ['200', '201', '202', '204']) {
		if (operation.responses?.[code]) {
			return operation.responses[code].content?.['application/json']?.schema;
		}
	}
	return undefined;
}

function dereference(schema) {
	if (!schema?.$ref) return schema;
	const name = schema.$ref.split('/').at(-1);
	return spec.components.schemas[name];
}

const lines = [
	'// Generated from api/openapi.json by scripts/generate-api-client.mjs.',
	'// Do not edit by hand; run `npm run generate:api` from web/.',
	''
];

for (const [name, schema] of Object.entries(spec.components.schemas)) {
	lines.push(`export type ${name} = ${typeOf(schema)};`, '');
}

const operations = [];
const operationIds = new Set();
for (const [path, pathItem] of Object.entries(spec.paths)) {
	for (const method of ['get', 'post', 'put', 'patch', 'delete']) {
		const operation = pathItem[method];
		if (!operation) continue;
		if (!operation.operationId) throw new Error(`${method.toUpperCase()} ${path} is missing operationId`);
		if (operationIds.has(operation.operationId)) throw new Error(`Duplicate operationId: ${operation.operationId}`);
		operationIds.add(operation.operationId);
		const requestSchema = operation.requestBody?.content?.['application/json']?.schema;
		const resolvedRequest = dereference(requestSchema);
		if (resolvedRequest && (resolvedRequest.type === 'object' || resolvedRequest.properties) && resolvedRequest.additionalProperties !== false) {
			throw new Error(`${operation.operationId} request schema must set additionalProperties=false`);
		}
		const parameters = [...(pathItem.parameters || []), ...(operation.parameters || [])];
		const parameterSchema = parameters.length ? {
			type: 'object',
			required: parameters.filter((parameter) => parameter.required).map((parameter) => parameter.name),
			properties: Object.fromEntries(parameters.map((parameter) => [parameter.name, parameter.schema]))
		} : undefined;
		operations.push({
			id: operation.operationId,
			method: method.toUpperCase(),
			path,
			pathParams: parameters.filter((parameter) => parameter.in === 'path').map((parameter) => parameter.name),
			queryParams: parameters.filter((parameter) => parameter.in === 'query').map((parameter) => parameter.name),
			parameters: typeOf(parameterSchema),
			body: typeOf(requestSchema),
			response: typeOf(responseSchema(operation))
		});
	}
}

lines.push('export interface Operations {');
for (const operation of operations) {
	lines.push(`\t${operation.id}: {`, `\t\tparameters: ${operation.parameters};`, `\t\trequestBody: ${operation.body};`, `\t\tresponse: ${operation.response};`, '\t};');
}
lines.push('}', '');
lines.push('export const operationDefinitions = {');
for (const operation of operations) {
	lines.push(`\t${operation.id}: ${JSON.stringify({ method: operation.method, path: operation.path, pathParams: operation.pathParams, queryParams: operation.queryParams })},`);
}
lines.push('} as const;', '');

const generated = lines.join('\n');
if (process.argv.includes('--check')) {
	const current = await readFile(outputPath, 'utf8').catch(() => '');
	if (current !== generated) {
		throw new Error('Generated API client is stale. Run `npm run generate:api` from web/.');
	}
} else {
	await writeFile(outputPath, generated);
}
