## NULL and MISSING values

JSON documents can contain explicit NULL values, and can also omit
fields entirely.  The IS [ NOT ] NULL / MISSING family of operators
let you test these conditions.

The query on the right looks for people where the children field is
explicitly set to NULL.

Try changing the query to IS MISSING.

<pre id="example">
	SELECT fname, children
		FROM tutorial 
			WHERE children IS NULL
</pre>
