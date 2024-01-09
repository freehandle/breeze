package main

const blocklist = `
<!DOCTYPE html>
<html>
<head>
	<title>Breeze Block Explorer</title>
</head>
<body>
	<h1>Breeze Block Explorer</h1>
	<h2>Blocks</h2>
	<table>
		<thead>
			<tr>
				<th>Epoch</th>
				<th>Published At</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tobdy>
			{{range .Blocks}}
			<tr>
				<td> <a href="/block/{{.Hash}}">{{.Epoch}}</a> </td>
				<td> {{.Published}} </td>
				<td> {{.Actions}} </td>
			</tr>
			{{end}}
		</tobdy>
	</table>
</body>
</html>
`

const blockdetail = `
<!DOCTYPE html>
<html>
<head>
	<title>Breeze Block Explorer</title>
</head>
<body>
	<h1>Breeze Block Explorer</h1>
	<h2>Blocks</h2>
	<table>
		<thead>
			<tr>
				<th>Epoch</th>
				<th>Published At</th>
				<th>Actions</th>
			</tr>
		</thead>
		<tobdy>
			{{range .Blocks}}
			<tr>
				<td> <a href="/block/{{.Hash}}">{{.Epoch}}</a> </td>
				<td> {{.Published}} </td>
				<td> {{.Actions}} </td>
			</tr>
			{{end}}
		</tobdy>
	</table>
</body>
</html>

`
